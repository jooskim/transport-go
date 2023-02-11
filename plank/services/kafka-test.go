package services

import (
	"encoding/json"
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/google/uuid"
	"github.com/vmware/transport-go/bus"
	"github.com/vmware/transport-go/model"
	"github.com/vmware/transport-go/plank/utils"
	"github.com/vmware/transport-go/service"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const KafkaTestChannel = "kafka-test"
const KafkaConfigFile = "kafka.json"

type KafkaTestService struct {
	producer   *kafka.Producer
	svcCore    service.FabricServiceCore
	activeSubs map[string]*kafkaSubscription
	readyChan  chan bool
	mu         sync.RWMutex
}

func NewKafkaTestService() *KafkaTestService {
	return &KafkaTestService{
		readyChan:  make(chan bool, 1),
		activeSubs: make(map[string]*kafkaSubscription),
	}
}

func (k *KafkaTestService) Init(core service.FabricServiceCore) (err error) {
	k.svcCore = core
	if err = k.genProducer(KafkaConfigFile); err != nil {
		return err
	}

	if err = k.monitorProduceEvents(); err != nil {
		return err
	}
	k.readyChan <- true
	return nil
}

func (k *KafkaTestService) OnServiceReady() chan bool {
	return k.readyChan
}

func (k *KafkaTestService) OnServerShutdown() {
	k.producer.Close()
	return
}

func (ps *KafkaTestService) GetRESTBridgeConfig() []*service.RESTBridgeConfig {
	return []*service.RESTBridgeConfig{
		{
			ServiceChannel: KafkaTestChannel,
			Uri:            "/kafka/generate-sample-messages",
			Method:         http.MethodPost,
			AllowHead:      false,
			AllowOptions:   false,
			FabricRequestBuilder: func(w http.ResponseWriter, r *http.Request) model.Request {
				body, _ := io.ReadAll(r.Body)
				return model.CreateServiceRequest("generateSampleMessages", body)
			},
		},
		{
			ServiceChannel: KafkaTestChannel,
			Uri:            "/kafka/subscribe-to-topic",
			Method:         http.MethodGet,
			FabricRequestBuilder: func(w http.ResponseWriter, r *http.Request) model.Request {
				topicName := r.URL.Query().Get("topicName")
				newId := uuid.New()
				return model.Request{
					Id:      &newId,
					Request: "subToTopic",
					Payload: topicName,
				}
			},
		},
	}
}

type kafkaSubscription struct {
	topics   []string
	consumer *kafka.Consumer
	core     service.FabricServiceCore
	busChan  *bus.Channel
	open     bool
}

func (sub *kafkaSubscription) unsubscribe() error {
	if !sub.open {
		return fmt.Errorf("already closed")
	}
	defer func() {
		sub.open = false
		sub.core.Bus().GetChannelManager().DestroyChannel(sub.busChan.Name)
	}()
	return sub.consumer.Close()
}

func (sub *kafkaSubscription) getBusChannelName() string {
	return sub.busChan.Name
}

func (sub *kafkaSubscription) subscribe() error {
	err := sub.consumer.SubscribeTopics(sub.topics, nil)
	if err != nil {
		return err
	}

	sub.open = true
	go func() {
		var err error
		var msg *kafka.Message
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
		run := true
		for run {
			select {
			case sig := <-c:
				utils.Log.Infof("received termination signal. closing kafka subscription %v\n", sig)
				run = false
			default:
				msg, err = sub.consumer.ReadMessage(100 * time.Millisecond)
				if err != nil {
					continue
				}
				if _, err = sub.consumer.CommitMessage(msg); err != nil {
					utils.Log.Errorln(err)
					continue
				}
				utils.Log.Debugf("message arrived from topic %s: key = %-10s value = %s\n", *msg.TopicPartition.Topic, string(msg.Key), string(msg.Value))
				msgId := uuid.New()
				sub.busChan.Send(model.GenerateResponse(&model.MessageConfig{
					Id:        &msgId,
					Channel:   sub.busChan.Name,
					Payload:   fmt.Sprintf("%s: %s", string(msg.Key), string(msg.Value)),
					Direction: 1,
				}))
			}
		}
		_ = sub.unsubscribe()
	}()
	return err
}

func newKafkaSubscription(topics []string, core service.FabricServiceCore) (*kafkaSubscription, error) {
	var err error
	s := &kafkaSubscription{
		topics: topics,
		core:   core,
	}
	id := uuid.New()
	s.busChan = core.Bus().GetChannelManager().CreateChannel(id.String())
	s.busChan.SetPrivate(true)
	s.consumer, err = getConsumer(KafkaConfigFile)
	if err != nil {
		return nil, err
	}

	return s, err
}

func (k *KafkaTestService) HandleServiceRequest(request *model.Request, core service.FabricServiceCore) {
	switch request.Request {
	case "generateSampleMessages":
		requestBody := make(map[string]any)
		_ = json.Unmarshal(request.Payload.([]byte), &requestBody)
		topicName, ok := requestBody["topicName"].(string)
		if !ok {
			topicName = "hello"
		}
		err := k.generateSampleMessages(topicName)
		if err != nil {
			core.SendErrorResponse(request, 400, err.Error())
		}
		core.SendResponse(request, map[string]any{
			"success":   true,
			"timestamp": time.Now().Unix(),
		})
	case "subToTopic":
		subTopics := request.Payload.(string)
		newSub, err := newKafkaSubscription(strings.Split(subTopics, ","), core)
		if err != nil {
			core.SendErrorResponse(request, 400, err.Error())
			return
		}

		if err = newSub.subscribe(); err != nil {
			core.SendErrorResponse(request, 400, err.Error())
			return
		}

		k.mu.Lock()
		k.activeSubs[newSub.getBusChannelName()] = newSub
		k.mu.Unlock()

		core.SendResponse(request, map[string]any{
			"fabricTopic": newSub.getBusChannelName(),
			"timestamp":   time.Now().Unix(),
		})
	default:
		core.SendErrorResponse(request, 400, "unknown command")
	}
}

func (k *KafkaTestService) genProducer(configFile string) error {
	config := kafka.ConfigMap{}
	raw, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &config); err != nil {
		return err
	}

	// create a sample producer w/ config
	k.producer, err = kafka.NewProducer(&config)
	return err
}

func getConsumer(configFile string) (*kafka.Consumer, error) {
	var consumer *kafka.Consumer
	config := kafka.ConfigMap{}
	raw, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &config); err != nil {
		return nil, err
	}

	config["enable.auto.commit"] = "false"
	config["auto.offset.reset"] = "earliest"
	consumer, err = kafka.NewConsumer(&config)
	return consumer, err
}

func (k *KafkaTestService) monitorProduceEvents() error {
	if k.producer == nil {
		return fmt.Errorf("no active producer found")
	}
	go func() {
		utils.Log.Infoln("started listening for produce message events")
		for evt := range k.producer.Events() {
			switch ev := evt.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					utils.Log.Errorf("failed to deliver msg: %v\n", ev.TopicPartition)
				} else {
					utils.Log.Infof("produced event to topic %s: key = %-10s value = %s\n", *ev.TopicPartition.Topic, string(ev.Key), string(ev.Value))
				}
			}
		}
	}()
	return nil
}

func (k *KafkaTestService) generateSampleMessages(topic string) error {
	if k.producer == nil {
		return fmt.Errorf("no active producer found")
	}
	users := [...]string{"eabara", "jsmith", "sgarcia", "jbernard", "htanaka", "awalther"}
	items := [...]string{"book", "alarm clock", "t-shirts", "gift card", "batteries"}

	for n := 0; n < 10; n++ {
		key := users[rand.Intn(len(users))]
		data := items[rand.Intn(len(items))]
		k.producer.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
			Key:            []byte(key),
			Value:          []byte(data),
		}, nil)
	}
	k.producer.Flush(int(10 * time.Second.Milliseconds()))
	return nil
}
