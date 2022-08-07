package rabmq

import (
	"encoding/json"
	"fmt"
	"github.com/streadway/amqp"
	"go.uber.org/zap"
	"joygosrv/share/log"
	"math/rand"
	"strconv"
	"time"
)

func MqTest(AmqpURL *string) {
	logger, err := log.NewZapLogger()
	if err != nil {
		fmt.Printf("cannot new logger:%v", err)
	}
	amqpConn, err := amqp.Dial(*AmqpURL)
	if err != nil {
		logger.Fatal("cannot connect to mq:%v", zap.Error(err))
	}
	//defer amqpConn.Close()
	go func() {
		pu, clearUp, err := NewPublisher(amqpConn, logger)
		if err != nil {
			logger.Fatal("new publisher err", zap.Error(err))
		}
		cases := make([]string, 100)
		for i := 1; i <= 100; i++ {
			cases = append(cases, strconv.Itoa(i))
		}
		dt := struct {
			ID         string `json:"id"`
			SellCount  int64  `json:"sell_count"`
			Period     string `json:"period"`
			GoodType   string `json:"good_type"`
			Status     int 	  `json:"status"`
		}{
			ID:         "",
			SellCount: 1,
			Period:     "",
			GoodType:  "joy",
			Status:     1,
		}
		j := 1
		goods_id := []int{46,47,48,49,50,51,52,53,54,55,56,57,60,61,62,63,64,84,85,118,120,121,122,123,124,125,126,127}
		for {
			dt.ID = strconv.Itoa(goods_id[rand.Intn(27)])
			b, err := json.Marshal(dt)
			if err != nil {
				logger.Fatal("json marshal err", zap.Error(err))
			}
			err = pu.Publish("queue:joygo", b)
			if err != nil{
				goto error
			}
			time.Sleep(time.Millisecond * 1000)
			dt.SellCount++
			j++
		}
		error:
			clearUp()
			amqpConn.Close()
			logger.Error("mq publish goto error:",zap.Error(err))
	}()
}
