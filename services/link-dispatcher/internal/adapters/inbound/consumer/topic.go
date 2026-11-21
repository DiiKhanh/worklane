package consumer

import (
	"errors"

	"github.com/IBM/sarama"
)

// EnsureTopic applies the privacy retention policy before consumption. Seven-day
// retention leaves room below 30 days for segment rolling and broker deletion.
func EnsureTopic(brokers []string, topic string) error {
	admin, err := sarama.NewClusterAdmin(brokers, sarama.NewConfig())
	if err != nil {
		return err
	}
	defer admin.Close()
	settings := map[string]*string{}
	for key, value := range map[string]string{
		"cleanup.policy":         "delete",
		"retention.ms":           "604800000",
		"segment.ms":             "3600000",
		"file.delete.delay.ms":   "60000",
		"message.timestamp.type": "LogAppendTime",
	} {
		settings[key] = &value
	}
	err = admin.CreateTopic(topic, &sarama.TopicDetail{NumPartitions: 1, ReplicationFactor: 1, ConfigEntries: settings}, false)
	var topicErr *sarama.TopicError
	if err != nil && (!errors.As(err, &topicErr) || topicErr.Err != sarama.ErrTopicAlreadyExists) {
		return err
	}
	if err != nil {
		updates := make(map[string]sarama.IncrementalAlterConfigsEntry, len(settings))
		for key, value := range settings {
			updates[key] = sarama.IncrementalAlterConfigsEntry{Operation: sarama.IncrementalAlterConfigsOperationSet, Value: value}
		}
		return admin.IncrementalAlterConfig(sarama.TopicResource, topic, updates, false)
	}
	return nil
}
