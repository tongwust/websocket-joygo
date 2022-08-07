package ws

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/streadway/amqp"
	"go.uber.org/zap"
	"joygosrv/mq"
	"joygosrv/mq/rabmq"
	"joygosrv/share/common"
	"log"
	"net/http"
	"sync"
	"time"
)

type Ws struct {
	MsgChans map[string]chan interface{}
	InChan   chan *MqMessage
	TagUser  map[string][]string
	UserConn map[string]*Cn
	//Clients  map[string]map[string]*websocket.Conn
	Mux      *sync.RWMutex
	Upgrader *websocket.Upgrader
	Logger   *zap.Logger
}

type Cn struct {
	Id   string
	Conn *websocket.Conn
	//closeChan chan byte
	isClosed bool
}

type MqMessage struct {
	messageType int
	data        *Data
	id          string
	userId      string
}

type Data struct {
	Id string `json:"id"`
}

func (ws *Ws) WsReadLoop() {
	for {
		for userId, cli := range ws.UserConn {
			_, data, err := cli.Conn.ReadMessage()
			if err != nil {
				ws.Logger.Error("read loop readmsg err:", zap.Error(err), zap.String("id", cli.Id), zap.String("id", userId))
				return
			}
			fmt.Printf("get msg:%s\n", string(data))
			var d Data
			if err := json.Unmarshal(data, &d); err != nil {
				ws.Logger.Error("json unmarshal err:", zap.Error(err), zap.String("id", cli.Id), zap.String("id", userId))
				return
			}
			if d.Id == cli.Id {
				ws.Logger.Info("no update d.Id:", zap.String("Id", d.Id))
				return
			}
			//ws.Mux.Lock()
			//if _,ok := ws.TagUser[d.data.Id];ok{
			fmt.Printf("userID:%s\n", userId)
			ws.TagUser[d.Id] = append(ws.TagUser[d.Id], userId)
			//}
			users, _ := ws.TagUser[cli.Id]
			for i, v := range users {
				if v == userId {
					ws.TagUser[cli.Id] = append(ws.TagUser[cli.Id][:i], ws.TagUser[cli.Id][i+1:]...)
					break
				}
			}
			if _, ok := ws.UserConn[userId]; ok {
				ws.UserConn[userId].Id = d.Id
			}
			//ws.Mux.Unlock()
			if ch, exist := ws.getMsgChannel(d.Id); !exist {
				ch = make(chan interface{})
				ws.addMsgChannel(d.Id, ch)
			}
			fmt.Printf("taguser2=%+v\n", ws.TagUser)
			fmt.Printf("UserConn2=%+v\n", ws.UserConn[userId])
		}
	}
}

func (ws *Ws) WsProcessLoop() {
	var forever chan struct{}
	for {
		select {
		case d := <-ws.InChan:
			ws.Logger.Info("switch id", zap.String("newId", d.data.Id), zap.String("oldId", d.id), zap.String("userId", d.userId))
			ws.Mux.Lock()
			if _, ok := ws.TagUser[d.id]; ok {
				for i, v := range ws.TagUser[d.id] {
					if v == d.userId {
						ws.TagUser[d.id] = append(ws.TagUser[d.id][:i], ws.TagUser[d.id][i+1:]...)
						//break   避免重复连接未释放，删除重复
					}
				}
			}
			if _, ok := ws.UserConn[d.userId]; ok {
				ws.UserConn[d.userId].Id = d.data.Id
				ws.TagUser[d.data.Id] = append(ws.TagUser[d.data.Id], d.userId)
			}
			ws.Mux.Unlock()
			//if ch, exist := ws.getMsgChannel(d.data.Id); !exist {
			//	ch = make(chan interface{})
			//	ws.addMsgChannel(d.data.Id, ch)
			//}
			ws.Logger.Info("taguser-map", zap.Any("tagUser", ws.TagUser))
			ws.Logger.Info("user-conn", zap.Any("UserConn", ws.UserConn))
		case <-forever:

		}
	}
}

func (ws *Ws) MsgHandle(c *gin.Context) {
	id := c.Param("id")
	if id != "" {
		_, exist := ws.getMsgChannel(id)
		if !exist {
			c.JSON(http.StatusNotFound, gin.H{
				"message": fmt.Sprintf("not exist this client %s", id),
			})
			return
		}
	}
	var m interface{}
	if err := c.BindJSON(&m); err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "message set failed",
		})
		return
	}
	if id == "" {
		ws.setMsgAllClient(m)
	} else {
		ws.setMsg(id, m)
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "send success",
	})
	return
}

func (ws *Ws) MqMsgHandle(msg amqp.Delivery) {
	var d mq.Data
	err := json.Unmarshal(msg.Body, &d)
	if err != nil {
		ws.Logger.Error("cannot unmarshal", zap.Error(err))
	} else {
		Ids := make([]string,0,len(ws.TagUser)+2)
		if d.T == 0{
			Ids = []string{"all", d.Id}
		}else{
			tUser := ws.TagUser
			for k,_ := range tUser{
				Ids = append(Ids,k)
			}
		}
		ws.Logger.Info("ids",zap.Any("ids",Ids),zap.Int("ids len:",len(Ids)))
		if len(Ids) > 0{
			for _, id := range Ids {
				ws.Logger.Info("id",zap.Any("id",id))
				ws.Mux.RLock()
				users, ok := ws.TagUser[id]
				ws.Mux.RUnlock()
				if ok {
					if len(users) == 0{
						ws.Mux.Lock()
						if len(ws.TagUser[id]) == 0{
							delete(ws.TagUser,id)
						}
						ws.Mux.Unlock()
						continue
					}
					ws.Logger.Info("users",zap.Any("users",users))
					for i := 0;i < len(users); {
						ws.Mux.RLock()
						cn, ok := ws.UserConn[users[i]]
						ws.Mux.RUnlock()
						if ok && cn.Id == id {
							ws.Logger.Info("id",zap.String("id",id),zap.String("userId",users[i]))
							cn.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
							err := cn.Conn.WriteMessage(websocket.TextMessage, msg.Body)
							if err != nil {
								ws.Logger.Error("write json err", zap.Error(err), zap.String("userId", users[i]))
								ws.DeleteClient(users[i])
							}else{
								i++
							}
						}else{
							ws.Mux.Lock()
							if i+1 <= len(ws.TagUser[id]) {
								ws.TagUser[id] = append(ws.TagUser[id][:i], ws.TagUser[id][i+1:]...)
								ws.Logger.Info("users",zap.Any("users1111",ws.TagUser[id]))
							}else{
								i++
							}
							ws.Mux.Unlock()
						}
					}
				}
			}
		}
		Ids = nil
	}
	if err = msg.Ack(false); err != nil {
		ws.Logger.Error("ack msg err:", zap.Error(err))
	}
	//time.Sleep(time.Millisecond * 100)
	return
}

// WsHandler send msg
func (ws *Ws) WsHandler(w http.ResponseWriter, r *http.Request, id string, userId string) {
	var conn *websocket.Conn
	var err error
	//pingTicker := time.NewTicker(time.Second * 10)
	conn, err = ws.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		ws.Logger.Error("Failed to set websocket upgrade", zap.Error(err))
		return
	}
	ws.addClient(id, userId, conn)
	conn.SetCloseHandler(func(code int, text string) error {
		if code == 1000 {
			ws.DeleteClient(userId)
			ws.Logger.Info("set close handler", zap.Int("code", code), zap.String("text", text))
		}
		return nil
	})
	go func(userId string) {
		defer func() {
			if err := recover(); err != nil{
				ws.Logger.Error("goroutine panic:",zap.Any("panic",err), zap.String("userId", userId))
			}
		}()
		for {
			// 读一个message
			ws.Mux.RLock()
			cn := ws.UserConn[userId]
			ws.Mux.RUnlock()
			if cn == nil || cn.isClosed {
				ws.Logger.Info("close goroutine")
				goto closed
			}
			err := cn.Conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
			if err != nil{
				goto error
			}
			_, data, err := cn.Conn.ReadMessage()
			if data != nil{
				ws.Logger.Info("read msg:", zap.ByteString("new id-1", data))
			}
			if err != nil {
				if websocket.IsCloseError(err,1000,1001,1002,1003,1006,1007,1008,1011,1012,1014,1015) {
					ws.Logger.Info("websocket close:", zap.String("userId", userId), zap.Error(err))
				}else{
					ws.Logger.Error("get msg err:", zap.Error(err), zap.String("userId", userId))
				}
				goto error
				//str := err.Error()
				//if ok := strings.Contains(str,"i/o timeout");ok{
				//	time.Sleep(500 * time.Millisecond)
				//	continue
				//}else{
				//	ws.Logger.Info("websocket close:", zap.String("userId", userId),zap.Error(err))
				//	goto error
				//}
				//if websocket.IsCloseError(err,1000,1001,1002,1003,1006,1007,1008,1011,1012,1014,1015) {
				//	ws.Logger.Info("websocket close:", zap.String("userId", userId),zap.Error(err))
				//	goto error
				//} else {
				//	time.Sleep(500 * time.Millisecond)
				//	continue
				//}
			}
			//ws.Logger.Info("read msg:", zap.ByteString("new id-1", data))
			var d Data
			if err := json.Unmarshal(data, &d); err != nil {
				ws.Logger.Error("json unmarshal err:", zap.Error(err), zap.String("id", cn.Id), zap.String("userId", userId), zap.ByteString("data", data))
				continue
			}
			//ws.Logger.Info("read msg:",zap.ByteString("new id-22",data))
			if d.Id == cn.Id || len(d.Id) == 0 {
				continue
			}
			//ws.Logger.Info("read msg:",zap.ByteString("new id-444",data))
			//req := &MqMessage{
			//	messageType: msgType,
			//	data:        &d,
			//	id:          cn.Id,
			//	userId:      userId,
			//}
			ws.Mux.RLock()
			users, ok := ws.TagUser[cn.Id]
			ws.Mux.RUnlock()
			ws.Logger.Info("users",zap.Any("users",users))
			if ok {
				for i := 0;i < len(users); {
					if users[i] == userId {
						ws.Mux.Lock()
						if i+1 <= len(ws.TagUser[cn.Id]){
							ws.TagUser[cn.Id] = append(ws.TagUser[cn.Id][:i], ws.TagUser[cn.Id][i+1:]...)
							ws.Logger.Info("users",zap.Any("users2222",ws.TagUser[cn.Id]))
						}
						ws.Mux.Unlock()
						break
					}else{
						i++
					}
				}
			}
			ws.Mux.RLock()
			_, ok = ws.UserConn[userId]
			ws.Mux.RUnlock()
			if ok {
				ws.Mux.Lock()
				ws.UserConn[userId].Id = d.Id
				if ok := common.InSlice(ws.TagUser[d.Id], userId); !ok {
					ws.TagUser[d.Id] = append(ws.TagUser[d.Id], userId)
					ws.Logger.Info("users",zap.Any("users3333",ws.TagUser[d.Id]))
				}
				ws.Mux.Unlock()
			}
			//ws.Logger.Info("taguser-map",zap.Any("tagUser",ws.TagUser))
			//ws.Logger.Info("user-conn",zap.Any("UserConn",ws.UserConn))
			//select {
			//case <-cn.closeChan:
			//	goto closed
			//}
			// 放入请求队列
			//select {
			//case ws.InChan <- req:
			//	ws.Logger.Info("read msg:",zap.ByteString("new id-3",data))
			//case <-cn.closeChan:
			//	goto closed
			//case <-pingTicker.C:
			//	if cn, ok := ws.UserConn[userId]; ok {
			//		cn.Conn.SetWriteDeadline(time.Now().Add(time.Second * 20))
			//		if err := cn.Conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
			//			ws.Logger.Error("ping msg err:", zap.Error(err),zap.String("userId",userId))
			//			goto error
			//		}
			//	}
			//}
		}
	error:
		ws.DeleteClient(userId)
	closed:
	}(userId)
}

func (ws *Ws) ConsumeMq(qn string, AmqpURL *string) {
	amqpConn, err := amqp.Dial(*AmqpURL)
	if err != nil {
		ws.Logger.Fatal("cannot dial amqp", zap.Error(err))
	}
	sub, err := rabmq.NewSubscriber(amqpConn, ws.Logger)
	if err != nil {
		ws.Logger.Fatal("cannot create subscriber", zap.Error(err))
	}
	amqpCh, clearUp, err := sub.Subscribe(qn)
	defer clearUp()
	if err != nil {
		ws.Logger.Fatal("subscribe err:", zap.Error(err))
	}
	var forever chan struct{}
	go func() {
		for msg := range amqpCh {
			ws.Logger.Info("", zap.ByteString("msg", msg.Body))
			ws.MqMsgHandle(msg)
		}
	}()
	ws.Logger.Info("waiting for msg *", zap.String("qname", qn))
	go sub.ReConnect(AmqpURL)
	<-forever
}

func (ws *Ws) addClient(id string, userId string, conn *websocket.Conn) {

	//if ws.Clients[id] == nil {
	//	ws.Clients[id] = make(map[string]*websocket.Conn)
	//}
	if ws.TagUser == nil {
		ws.TagUser = make(map[string][]string,1000)
	}
	ws.Mux.Lock()
	for k,_ := range ws.TagUser{
		if k != id{
			ws.TagUser[k] = common.ClearRepSlice(ws.TagUser[k])
		}
	}
	ws.Mux.Unlock()
	ws.Mux.RLock()
	users ,_ := ws.TagUser[id]
	ws.Mux.RUnlock()
	if ex := common.InSlice(users, userId); !ex {
		ws.Mux.Lock()
		ws.TagUser[id] = append(ws.TagUser[id], userId)
		ws.Logger.Info("users",zap.Any("users4444",ws.TagUser[id]))
		ws.Mux.Unlock()
	}
	if ws.UserConn == nil {
		ws.UserConn = make(map[string]*Cn,1000)
	}
	ws.Mux.Lock()
	ws.UserConn[userId] = &Cn{
		Conn: conn,
		Id:   id,
		//closeChan: make(chan byte),
		isClosed: false,
	}
	ws.Mux.Unlock()
	//ws.Logger.Info("user-conn",zap.Any("tagUser",ws.UserConn))
}

func (ws *Ws) GetClient(id string, userId string) (*websocket.Conn, bool) {
	ws.Mux.Lock()
	//conn, exist = ws.Clients[id][userId]
	cn, exist := ws.UserConn[userId]
	ws.Mux.Unlock()
	return cn.Conn, exist
}

func (ws *Ws) DeleteClient(userId string) {
	ws.Mux.RLock()
	cn, ok := ws.UserConn[userId]
	ws.Mux.RUnlock()
	if !ok {
		ws.Logger.Warn("cannot find userConn userId", zap.String("userId", userId))
	} else {
		cn.Conn.Close()
		//if !ws.UserConn[userId].isClosed{
		cn.isClosed = true
		//close(ws.UserConn[userId].closeChan)
		//}
		ws.Mux.RLock()
		users, ok := ws.TagUser[cn.Id]
		ws.Mux.RUnlock()
		if !ok {
			ws.Logger.Warn("cannot find TagUser", zap.String("Id", ws.UserConn[userId].Id), zap.String("userId", userId))
		} else {
			for i := 0; i < len(users); {
				if users[i] == userId {
					ws.Mux.Lock()
					if i+1 <= len(ws.TagUser[cn.Id]){
						ws.TagUser[cn.Id] = append(ws.TagUser[cn.Id][:i], ws.TagUser[cn.Id][i+1:]...)
						ws.Logger.Info("users",zap.Any("users5555",ws.TagUser[cn.Id]))
					}else{
						i++
					}
					ws.Mux.Unlock()
					//break //去掉冗余数据
				}else{
					i++
				}
			}
		}
		ws.Mux.Lock()
		delete(ws.UserConn, userId)
		ws.Mux.Unlock()
	}
}

func (ws *Ws) addMsgChannel(id string, m chan interface{}) {
	ws.Mux.Lock()
	ws.MsgChans[id] = m
	ws.Mux.Unlock()
}

func (ws *Ws) getMsgChannel(id string) (m chan interface{}, exist bool) {
	ws.Mux.Lock()
	m, exist = ws.MsgChans[id]
	ws.Mux.Unlock()
	return
}

func (ws *Ws) setMsg(id string, content interface{}) {
	ws.Mux.Lock()
	defer ws.Mux.Unlock()
	if m, exist := ws.MsgChans[id]; exist {
		go func() {
			m <- content
		}()
	} else if id == "all" {
		ch := make(chan interface{})
		go ws.addMsgChannel(id, ch)
		go func() {
			ch <- content
		}()
	}
}

func (ws *Ws) setMsgAllClient(content interface{}) {
	ws.Mux.Lock()
	all := ws.MsgChans
	ws.Mux.Unlock()
	go func() {
		for _, m := range all {
			m <- content
		}
	}()
}

func (ws *Ws) deleteMsgChannel(id string) {
	if id != "all" {
		ws.Mux.Lock()
		if ch, ok := ws.MsgChans[id]; ok {
			close(ch)
			delete(ws.MsgChans, id)
		}
		ws.Mux.Unlock()
	}
}
