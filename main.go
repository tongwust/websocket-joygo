package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/namsral/flag"
	"joygosrv/mq/rabmq"
	"joygosrv/share/common"
	"joygosrv/share/log"
	"joygosrv/share/verify"
	"joygosrv/ws"
	"net/http"
	"sync"
	"time"
)

var (
	addr     = flag.String("addr", ":8090", "server addr listen")
	AmqpURL  = flag.String("amqp_url", "amqp://admin:admin@localhost:5672/", "amqp url")
	certFile = flag.String("cert_file","/www/fullchain.pem","cert file")
	keyFile  = flag.String("key_file","/www/privkey.pem","key file")
	isTest	 = flag.String("istest","1","test write to amqp")
)

func main() {
	flag.Parse()
	logger, err := log.NewZapLogger()
	if err != nil {
		fmt.Printf("can not init logger:%v", err)
		return
	}
	r := gin.Default()
	wbs := ws.Ws{
		//MsgChanAll: make(chan interface{}),
		MsgChans:   make(map[string]chan interface{}),
		InChan:		make(chan *ws.MqMessage),
		TagUser: 	make(map[string][]string,1000),
		UserConn: 	make(map[string]*ws.Cn,1000),
		//Clients:    make(map[string]map[string]*websocket.Conn),
		Mux:        &sync.RWMutex{},
		Logger: 	logger,
		Upgrader: &websocket.Upgrader{
			ReadBufferSize:   1024,
			WriteBufferSize:  1024,
			HandshakeTimeout: 5 * time.Second,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
	// connect ws by group
	r.GET("/client/:id/:userId/:token", func(c *gin.Context) {
		id := c.Param("id")
		userId := c.Param("userId")
		mp := map[string]string{"id":id,"userId":userId}
		if ok := verify.Verify(mp,c.Param("token")); !ok || len(id) == 0 || len(userId) == 0{
			c.JSON(http.StatusUnauthorized, gin.H{
				"message": fmt.Sprintf("token err"),
			})
			return
		}
		wbs.WsHandler(c.Writer, c.Request, id, userId)
	})
	r.GET("/client/info", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"tagUser": fmt.Sprintf("%+v",wbs.TagUser),
			"userConn": fmt.Sprintf("%+v",wbs.UserConn),
		})
	})
	r.POST("/client/del/:userId", func(c *gin.Context) {
		wbs.DeleteClient(c.Param("userId"))
		c.JSON(http.StatusOK, gin.H{
			"tagUser": fmt.Sprintf("%+v",wbs.TagUser),
			"userConn": fmt.Sprintf("%+v",wbs.UserConn),
		})
	})
	r.POST("/client/clearrepeat/:id", func(c *gin.Context) {
		id := c.Param("id")
		wbs.Mux.RLock()
		_,ok := wbs.TagUser[id]
		wbs.Mux.RUnlock()
		if ok{
			wbs.Mux.Lock()
			wbs.TagUser[id] = common.ClearRepSlice(wbs.TagUser[id])
			wbs.Mux.Unlock()
		}
		c.JSON(http.StatusOK, gin.H{
			"tagUser": fmt.Sprintf("%+v",wbs.TagUser),
			"userConn": fmt.Sprintf("%+v",wbs.UserConn),
		})
	})
	r.POST("/client/clear/:id/:userId", func(c *gin.Context) {
		id := c.Param("id")
		userId := c.Param("userId")
		wbs.Mux.RLock()
		users,ok := wbs.TagUser[id]
		wbs.Mux.RUnlock()
		if ok{
			for i, v := range users {
				if v == userId {
					wbs.Mux.Lock()
					if i+1 <= len(wbs.TagUser[id]){
						wbs.TagUser[id] = append(wbs.TagUser[id][:i], wbs.TagUser[id][i+1:]...)
					}
					wbs.Mux.Unlock()
				}
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"tagUser": fmt.Sprintf("%+v",wbs.TagUser),
			"userConn": fmt.Sprintf("%+v",wbs.UserConn),
		})
	})
	r.POST("/client/clear/all", func(c *gin.Context) {
		wbs.Mux.RLock()
		for id,users := range wbs.TagUser{
			for i, userId := range users {
				if cn,ok := wbs.UserConn[userId];!ok || (ok && cn.Id != id){
					wbs.Mux.Lock()
					if i+1 <= len(wbs.TagUser[id]){
						wbs.TagUser[id] = append(wbs.TagUser[id][:i], wbs.TagUser[id][i+1:]...)
					}
					wbs.Mux.Unlock()
				}
			}
		}
		wbs.Mux.RUnlock()
		c.JSON(http.StatusOK, gin.H{
			"tagUser": fmt.Sprintf("%+v",wbs.TagUser),
			"userConn": fmt.Sprintf("%+v",wbs.UserConn),
		})
	})
	//go wbs.WsReadLoop()
	//go wbs.WsProcessLoop()

	//r.DELETE("/client/:id/:userId", func(c *gin.Context) {
	//	id := c.Param("id")
	//	userId := c.Param("userId")
	//	if conn, exist := wbs.GetClient(id, userId); exist {
	//		conn.Close()
	//		wbs.DeleteClient(userId)
	//	} else {
	//		c.JSON(http.StatusNotFound, gin.H{
	//			"message": fmt.Sprintf("client %s is not found!", id),
	//		})
	//	}
	//})
	Qnames := []string{"queue:joygo"}
	for _, name := range Qnames {
		go wbs.ConsumeMq(name,AmqpURL)
	}
	if *isTest == "1"{
		rabmq.MqTest(AmqpURL)
	}
	//r.POST("/message/:id", wbs.MsgHandle)
	//r.POST("/message", wbs.MsgHandle)
	logger.Sugar().Infof("server started at %s",*addr)
	//logger.Sugar().Infof("server err:",r.Run(*addr))
	logger.Sugar().Infof("server err:",r.RunTLS(*addr,*certFile,*keyFile))
}
