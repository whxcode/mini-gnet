package scheduler

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"
)

func _TestScheduler(t *testing.T) {
	scheduler := NewScheduler(runtime.NumCPU())

	scheduler.AddTask(
		NewTask("task1", func(ctx context.Context) (string, error) {
			panic("Simulated panic in task1") // Simulate a panic
			t.Log("Executing task1")
			return "task1 result", nil
		}),
	)

	scheduler.AddTask(
		NewTask("task2", func(ctx context.Context) (string, error) {
			time.Sleep(2 * time.Second) // Simulate a longer task
			t.Log("Executing task2")
			return "task2 result", nil
		}),
	)

	scheduler.AddTask(
		NewTask("task3", func(ctx context.Context) (string, error) {
			time.Sleep(1 * time.Second) // Simulate a longer task
			t.Log("Executing task3")
			return "task3 result", nil
		}),

		NewTask("task4", func(ctx context.Context) (string, error) {
			time.Sleep(1 * time.Second) // Simulate a longer task
			t.Log("Executing task4")
			return "task4 result", nil
		}),
	)

	scheduler.Close()
	scheduler.Wait()

	fmt.Println(scheduler.GetTaskSnapshot("task2"))
	fmt.Println(scheduler.GetTaskSnapshot("task1"))
}

func TestSchedulerCancel(t *testing.T) {
	scheduler := NewScheduler(1)

	scheduler.AddTask(
		NewTask("task1", func(ctx context.Context) (string, error) {
			panic("Simulated panic in task1") // Simulate a panic
			t.Log("Executing task1")
			return "task1 result", nil
		}),
	)

	scheduler.Close()
	scheduler.Wait()
}
