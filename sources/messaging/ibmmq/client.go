package ibmmq

import (
	"context"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/kubemq-hub/builder/connector/common"
	"github.com/kubemq-hub/ibmmq-sdk/mq-golang-jms20/jms20subset"
	"github.com/kubemq-hub/ibmmq-sdk/mq-golang-jms20/mqjms"
	"github.com/kubemq-hub/kubemq-sources/config"
	"github.com/kubemq-hub/kubemq-sources/middleware"
	"github.com/kubemq-hub/kubemq-sources/pkg/logger"
	"github.com/kubemq-hub/kubemq-sources/types"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type Client struct {
	name       string
	opts       options
	queue      jms20subset.Queue
	jmsContext jms20subset.JMSContext
	log        *logger.Logger
	consumer   jms20subset.JMSConsumer
}

func New() *Client {
	return &Client{}

}

func (c *Client) Connector() *common.Connector {
	return Connector()
}

func (c *Client) Init(ctx context.Context, cfg config.Spec) error {
	c.name = cfg.Name
	c.log = logger.NewLogger(cfg.Name)
	var err error
	c.opts, err = parseOptions(cfg)
	if err != nil {
		return err
	}
	cf := mqjms.ConnectionFactoryImpl{
		QMName:           c.opts.qMName,
		Hostname:         c.opts.hostname,
		PortNumber:       c.opts.portNumber,
		ChannelName:      c.opts.channelName,
		UserName:         c.opts.userName,
		TransportType:    c.opts.transportType,
		TLSClientAuth:    c.opts.tlsClientAuth,
		KeyRepository:    c.opts.keyRepository,
		Password:         c.opts.Password,
		CertificateLabel: c.opts.certificateLabel,
	}

	jmsContext, jmsErr := cf.CreateContext()
	if jmsErr != nil {
		return fmt.Errorf("failed to create context on error %s", jmsErr.GetReason())
	}
	c.jmsContext = jmsContext
	c.queue = jmsContext.CreateQueue(c.opts.queueName)
	c.consumer, jmsErr = c.jmsContext.CreateConsumer(c.queue)
	if jmsErr != nil {
		return fmt.Errorf("failed to create consumer on error %s", jmsErr.GetReason())
	}
	return nil
}

func (c *Client) createMetadataString(msg jms20subset.Message) string {
	md := map[string]string{}
	md["id"] = msg.GetJMSMessageID()
	md["timestamp"] = fmt.Sprintf("%d", msg.GetJMSTimestamp())
	str, err := json.MarshalToString(md)
	if err != nil {
		return fmt.Sprintf("error parsing jms20subset.message metadata, %s", err.Error())
	}
	return str
}

func (c *Client) Start(ctx context.Context, target middleware.Middleware) error {

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, jmsExp := c.consumer.Receive(c.opts.pullDelay)
				if msg != nil {
					switch msg := msg.(type) {
					case jms20subset.TextMessage:
						msgBodyStrPtr := msg.GetText()
						b, err := json.Marshal(*msgBodyStrPtr)
						if err != nil {
							c.log.Errorf("failed to parse message on error %s", err.Error())
						}
						req := types.NewRequest().SetMetadata(c.createMetadataString(msg)).SetData(b)
						_, err = target.Do(ctx, req)
						if err != nil {
							c.log.Errorf("error processing message %s", err.Error())
						}
					default:
						c.log.Error("is not a TextMessage")
					}
				}
				if jmsExp != nil {
					c.log.Errorf("error processing message %s", jmsExp.GetReason())
				}
			}
		}
	}()

	return nil
}

func (c *Client) Stop() error {
	if c.jmsContext != nil {
		c.jmsContext.Close()
	}
	return nil
}
