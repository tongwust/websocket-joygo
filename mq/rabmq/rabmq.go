package rabmq

import (
	"fmt"
	"github.com/streadway/amqp"
	"go.uber.org/zap"
)

// Publisher implements an ampq publisher.
type Publisher struct {
	ch       *amqp.Channel
	logger   *zap.Logger
}

// NewPublisher creates an amqp publisher
func NewPublisher(conn *amqp.Connection, logger *zap.Logger) (*Publisher,func(), error) {
	ch, err := conn.Channel()
	var closeCh = func() {
		ch.Close()
	}
	if err != nil {
		return nil, closeCh,fmt.Errorf("cannot allocate channel: %v", err)
	}
	return &Publisher{
		ch:       ch,
		logger: logger,
	}, closeCh,nil
}

// Publish publishes a message.
func (p *Publisher) Publish(qname string, b []byte) error {
	q, err := p.ch.QueueDeclare(
		qname, // name
		true,   // durable
		false,   // delete when unused
		false,   // exclusive
		false,   // no-wait
		nil,     // arguments
	)
	if err != nil{
		p.logger.Error("Failed to declare a queue",zap.Error(err))
		return err
	}
	return p.ch.Publish(
		"",
		q.Name,    // routing key
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			Body: b,
		},
	)
}

type Subscriber struct {
	conn     *amqp.Connection
	channel  *amqp.Channel
	logger   *zap.Logger
	connNotify chan *amqp.Error
	channelNotify chan *amqp.Error
	quit     chan struct{}
	consumerTag string
}

// NewSubscriber creates an amqp subscriber.
func NewSubscriber(conn *amqp.Connection, logger *zap.Logger) (*Subscriber, error) {
	return &Subscriber{
		conn:     conn,
		logger:   logger,
	}, nil
}

func (s *Subscriber)Subscribe( qname string) (<-chan amqp.Delivery,func(), error) {
	var err error
	s.channel, err = s.conn.Channel()
	if err != nil {
		return nil, func() {}, fmt.Errorf("cannot allocate channel: %v", err)
	}
	closeCh := func() {
		fmt.Printf("close ch:\n")
		err = s.conn.Close()
		if err != nil{
			s.logger.Error("cannot close amqp conn", zap.Error(err))
		}
		err := s.channel.Close()
		if err != nil {
			s.logger.Error("cannot close amqp channel", zap.Error(err))
		}
		s.quit <- struct{}{}
	}
	q, err := s.channel.QueueDeclare(
		qname,    // name
		true, // durable
		false,  // autoDelete
		false, // exlusive
		false, // noWait
		nil,   // args
	)
	if err != nil {
		return nil, closeCh, fmt.Errorf("cannot declare queue: %v", err)
	}
	s.consumerTag = qname
	msgs, err := s.channel.Consume(
		q.Name,
		s.consumerTag,    // consumer
		false,  // autoAck
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,   // args
	)
	if err != nil {
		return nil, closeCh, fmt.Errorf("cannot consume queue: %v", err)
	}
	s.connNotify = s.conn.NotifyClose(make(chan *amqp.Error))
	s.channelNotify = s.channel.NotifyClose(make(chan *amqp.Error))
	return msgs, closeCh, nil
}

// ReConnect not implement
func (s *Subscriber) ReConnect(AmqpURL *string) {
	select {
	case err := <-s.connNotify:
		if err != nil {
			s.logger.Error("rabbitmq consumer - connection NotifyClose: ", zap.Error(err))
		}
	case err := <-s.channelNotify:
		if err != nil {
			s.logger.Error("rabbitmq consumer - channel NotifyClose: ", zap.Error(err))
		}
	case <-s.quit:
		return
	}

	// backstop
	if !s.conn.IsClosed() {
		// 关闭 SubMsg message delivery
		if err := s.channel.Cancel(s.consumerTag, true); err != nil {
			s.logger.Error("rabbitmq consumer - channel cancel failed: ", zap.Error(err))
		}

		if err := s.conn.Close(); err != nil {
			s.logger.Error("rabbitmq consumer - conn close failed: ", zap.Error(err))
		}
	}

	// IMPORTANT: 必须清空 Notify，否则死连接不会释放
	for err := range s.channelNotify {
		s.logger.Error("channel notify err:",zap.Error(err))
	}
	for err := range s.connNotify {
		s.logger.Error("conn notify err:",zap.Error(err))
	}
	//quit:
	//	for {
	//		select {
	//		case <-s.quit:
	//			return
	//		default:
	//			s.logger.Error("rabbitmq consumer - reconnect")
	//			amqpConn, err := amqp.Dial(*AmqpURL)
	//			if err != nil {
	//				s.logger.Fatal("cannot dial amqp", zap.Error(err))
	//			}
	//			sub, err := NewSubscriber(amqpConn, s.logger)
	//			if err != nil {
	//				s.logger.Fatal("cannot create subscriber", zap.Error(err))
	//			}
	//			amqpCh, clearUp, err := sub.Subscribe(s.consumerTag)
	//			if err != nil {
	//				s.logger.Error("rabbitmq consumer - failCheck: ", err)
	//
	//				// sleep 5s reconnect
	//				time.Sleep(time.Second * 5)
	//				continue
	//			}
	//
	//			break quit
	//		}
	//	}
}

