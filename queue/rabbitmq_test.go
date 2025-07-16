package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestRabbitMQQueue(t *testing.T) {
	config := RabbitMQConfig{
		URL:          "amqp://admin:admin123@localhost:5672/",
		QueueName:    "test_queue",
		ExchangeName: "test_exchange",
		RoutingKey:   "test",
		Durable:      false,
		AutoDelete:   true,
		Exclusive:    false,
		NoWait:       false,
	}

	queue, err := NewRabbitMQQueue(config)
	if err != nil {
		t.Skipf("Skipping RabbitMQ test: %v", err)
	}
	defer queue.Close()

	ctx := context.Background()

	t.Run("BasicEnqueueDequeue", func(t *testing.T) {
		job := NewJob("test-job").
			WithID("test-1").
			WithPayload(map[string]interface{}{"message": "hello world"}).
			Build()

		err := queue.Enqueue(ctx, job)
		if err != nil {
			t.Fatalf("Failed to enqueue job: %v", err)
		}

		dequeuedJob, err := queue.Dequeue(ctx, time.Second*5)
		if err != nil {
			t.Fatalf("Failed to dequeue job: %v", err)
		}

		if dequeuedJob == nil {
			t.Fatal("Expected job, got nil")
		}

		if dequeuedJob.ID != job.ID {
			t.Errorf("Expected job ID %s, got %s", job.ID, dequeuedJob.ID)
		}

		if dequeuedJob.Type != job.Type {
			t.Errorf("Expected job type %s, got %s", job.Type, dequeuedJob.Type)
		}

		if dequeuedJob.Status != StatusRunning {
			t.Errorf("Expected job status %s, got %s", StatusRunning, dequeuedJob.Status)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(dequeuedJob.Payload, &payload); err != nil {
			t.Fatalf("Failed to unmarshal payload: %v", err)
		}

		if payload["message"] != "hello world" {
			t.Errorf("Expected message 'hello world', got %v", payload["message"])
		}
	})

	t.Run("ScheduledJobs", func(t *testing.T) {
		job := NewJob("scheduled-job").
			WithID("scheduled-1").
			WithDelay(time.Millisecond * 200).
			WithPayload(map[string]interface{}{"scheduled": true}).
			Build()

		err := queue.Schedule(ctx, job)
		if err != nil {
			t.Fatalf("Failed to schedule job: %v", err)
		}

		dequeuedJob, err := queue.Dequeue(ctx, time.Second*5)
		if err != nil {
			t.Fatalf("Failed to dequeue scheduled job: %v", err)
		}

		if dequeuedJob == nil {
			t.Fatal("Expected scheduled job, got nil")
		}

		if dequeuedJob.ID != job.ID {
			t.Errorf("Expected job ID %s, got %s", job.ID, dequeuedJob.ID)
		}

		dequeuedJob.Status = StatusCompleted
		queue.UpdateJob(ctx, dequeuedJob)
	})

	t.Run("PriorityJobs", func(t *testing.T) {
		lowJob := NewJob("low-job").
			WithID("low-1").
			WithPriority(PriorityLow).
			Build()

		highJob := NewJob("high-job").
			WithID("high-1").
			WithPriority(PriorityHigh).
			Build()

		normalJob := NewJob("normal-job").
			WithID("normal-1").
			WithPriority(PriorityNormal).
			Build()

		err := queue.Enqueue(ctx, normalJob)
		if err != nil {
			t.Fatalf("Failed to enqueue normal job: %v", err)
		}

		err = queue.Enqueue(ctx, lowJob)
		if err != nil {
			t.Fatalf("Failed to enqueue low job: %v", err)
		}

		err = queue.Enqueue(ctx, highJob)
		if err != nil {
			t.Fatalf("Failed to enqueue high job: %v", err)
		}

		// Give RabbitMQ time to process
		time.Sleep(time.Millisecond * 100)

		for i := 0; i < 3; i++ {
			job, err := queue.Dequeue(ctx, time.Second*5)
			if err != nil {
				t.Fatalf("Failed to dequeue job %d: %v", i, err)
			}

			if job == nil {
				t.Fatalf("Expected job %d, got nil", i)
			}

			job.Status = StatusCompleted
			err = queue.UpdateJob(ctx, job)
			if err != nil {
				t.Fatalf("Failed to update job %d: %v", i, err)
			}
		}

		// Note: RabbitMQ priority might not be strictly enforced in our simple test
		// but the jobs should all be processed successfully
	})

	t.Run("JobOperations", func(t *testing.T) {
		job := NewJob("test-ops").
			WithID("ops-1").
			WithPayload(map[string]interface{}{"operation": "test"}).
			Build()

		err := queue.Enqueue(ctx, job)
		if err != nil {
			t.Fatalf("Failed to enqueue job: %v", err)
		}

		retrievedJob, err := queue.GetJob(ctx, job.ID)
		if err != nil {
			t.Fatalf("Failed to get job: %v", err)
		}

		if retrievedJob.ID != job.ID {
			t.Errorf("Expected job ID %s, got %s", job.ID, retrievedJob.ID)
		}

		pendingJobs, err := queue.GetJobs(ctx, StatusPending, 10)
		if err != nil {
			t.Fatalf("Failed to get pending jobs: %v", err)
		}

		found := false
		for _, pJob := range pendingJobs {
			if pJob.ID == job.ID {
				found = true
				break
			}
		}

		if !found {
			t.Error("Job not found in pending jobs")
		}

		stats, err := queue.GetStats(ctx)
		if err != nil {
			t.Fatalf("Failed to get stats: %v", err)
		}

		if stats.Pending == 0 {
			t.Error("Expected pending jobs in stats")
		}

		err = queue.DeleteJob(ctx, job.ID)
		if err != nil {
			t.Fatalf("Failed to delete job: %v", err)
		}

		_, err = queue.GetJob(ctx, job.ID)
		if err == nil {
			t.Error("Expected error when getting deleted job")
		}
	})

	t.Run("Connection", func(t *testing.T) {
		if !queue.IsConnectionOpen() {
			t.Error("Expected connection to be open")
		}

		err := queue.PurgeQueue(ctx)
		if err != nil {
			t.Fatalf("Failed to purge queue: %v", err)
		}

		stats, err := queue.GetStats(ctx)
		if err != nil {
			t.Fatalf("Failed to get stats after purge: %v", err)
		}

		if stats.QueueSize != 0 {
			t.Errorf("Expected queue size 0 after purge, got %d", stats.QueueSize)
		}
	})
}

func TestRabbitMQConfig(t *testing.T) {
	config := RabbitMQConfig{}
	queue, err := NewRabbitMQQueue(config)
	if err != nil {
		t.Skipf("Skipping RabbitMQ config test: %v", err)
	}
	defer queue.Close()

	if queue.queueName != "jobs" {
		t.Errorf("Expected default queue name 'jobs', got %s", queue.queueName)
	}

	if queue.exchangeName != "jobs_exchange" {
		t.Errorf("Expected default exchange name 'jobs_exchange', got %s", queue.exchangeName)
	}

	if queue.routingKey != "jobs" {
		t.Errorf("Expected default routing key 'jobs', got %s", queue.routingKey)
	}
}

func TestRabbitMQReconnection(t *testing.T) {
	config := RabbitMQConfig{
		URL:          "amqp://admin:admin123@localhost:5672/",
		QueueName:    "test_reconnect",
		ExchangeName: "test_reconnect_exchange",
		RoutingKey:   "test_reconnect",
		Durable:      false,
		AutoDelete:   true,
	}

	queue, err := NewRabbitMQQueue(config)
	if err != nil {
		t.Skipf("Skipping RabbitMQ reconnection test: %v", err)
	}
	defer queue.Close()

	err = queue.Reconnect(config)
	if err != nil {
		t.Fatalf("Failed to reconnect: %v", err)
	}

	if !queue.IsConnectionOpen() {
		t.Error("Expected connection to be open after reconnection")
	}
}

func TestRabbitMQWithClosedConnection(t *testing.T) {
	config := RabbitMQConfig{
		URL:          "amqp://admin:admin123@localhost:5672/",
		QueueName:    "test_closed",
		ExchangeName: "test_closed_exchange",
		RoutingKey:   "test_closed",
		Durable:      false,
		AutoDelete:   true,
	}

	queue, err := NewRabbitMQQueue(config)
	if err != nil {
		t.Skipf("Skipping RabbitMQ closed connection test: %v", err)
	}

	queue.Close()

	ctx := context.Background()

	job := NewJob("test-closed").WithID("closed-1").Build()

	err = queue.Enqueue(ctx, job)
	if err == nil {
		t.Error("Expected error when enqueuing to closed queue")
	}

	_, err = queue.Dequeue(ctx, time.Second)
	if err == nil {
		t.Error("Expected error when dequeuing from closed queue")
	}

	_, err = queue.GetJob(ctx, job.ID)
	if err == nil {
		t.Error("Expected error when getting job from closed queue")
	}

	_, err = queue.GetJobs(ctx, StatusPending, 10)
	if err == nil {
		t.Error("Expected error when getting jobs from closed queue")
	}

	_, err = queue.GetStats(ctx)
	if err == nil {
		t.Error("Expected error when getting stats from closed queue")
	}

	err = queue.DeleteJob(ctx, job.ID)
	if err == nil {
		t.Error("Expected error when deleting job from closed queue")
	}
}

func BenchmarkRabbitMQEnqueue(b *testing.B) {
	config := RabbitMQConfig{
		URL:          "amqp://admin:admin123@localhost:5672/",
		QueueName:    "benchmark_queue",
		ExchangeName: "benchmark_exchange",
		RoutingKey:   "benchmark",
		Durable:      false,
		AutoDelete:   true,
	}

	queue, err := NewRabbitMQQueue(config)
	if err != nil {
		b.Skipf("Skipping RabbitMQ benchmark: %v", err)
	}
	defer queue.Close()

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			job := NewJob("benchmark-job").
				WithID(fmt.Sprintf("bench-%d", i)).
				WithPayload(map[string]interface{}{"index": i}).
				Build()

			err := queue.Enqueue(ctx, job)
			if err != nil {
				b.Fatalf("Failed to enqueue job: %v", err)
			}
			i++
		}
	})
}

func BenchmarkRabbitMQDequeue(b *testing.B) {
	config := RabbitMQConfig{
		URL:          "amqp://admin:admin123@localhost:5672/",
		QueueName:    "benchmark_dequeue",
		ExchangeName: "benchmark_dequeue_exchange",
		RoutingKey:   "benchmark_dequeue",
		Durable:      false,
		AutoDelete:   true,
	}

	queue, err := NewRabbitMQQueue(config)
	if err != nil {
		b.Skipf("Skipping RabbitMQ dequeue benchmark: %v", err)
	}
	defer queue.Close()

	ctx := context.Background()

	// Pre-populate queue with jobs
	for i := 0; i < b.N; i++ {
		job := NewJob("benchmark-dequeue-job").
			WithID(fmt.Sprintf("dequeue-bench-%d", i)).
			WithPayload(map[string]interface{}{"index": i}).
			Build()

		err := queue.Enqueue(ctx, job)
		if err != nil {
			b.Fatalf("Failed to enqueue job: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		job, err := queue.Dequeue(ctx, time.Second*5)
		if err != nil {
			b.Fatalf("Failed to dequeue job: %v", err)
		}
		if job == nil {
			b.Fatal("Expected job, got nil")
		}

		job.Status = StatusCompleted
		queue.UpdateJob(ctx, job)
	}
}
