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

func _TestSchedulerCancel(t *testing.T) {
	scheduler := NewScheduler(1)

	scheduler.AddTask(
		NewTask("task1", func(ctx context.Context) (string, error) {
			select {
			case <-ctx.Done():
				t.Log("task1 canceled")
				return "", ctx.Err()
			case <-time.After(5 * time.Second):
				fmt.Println("task1 completed")
				return "task1 result", nil
			}
		}),
	)

	scheduler.AddTask(
		NewTask("task2", func(ctx context.Context) (string, error) {
			select {
			case <-ctx.Done():
				t.Log("task2 canceled")
				return "", ctx.Err()
			case <-time.After(5 * time.Second):
				fmt.Println("task2 completed")
				return "task2 result", nil
			}
		}),
	)
	scheduler.Cancel("task2")

	time.Sleep(2 * time.Second) // Give the task some time to start
	scheduler.Cancel("task1")

	t.Log(scheduler.GetTaskSnapshot("task1"))

	err := scheduler.Cancel("task1")
	if err == nil {
		t.Error("Expected error when canceling a non-existent task, but got nil")
	}

	t.Log(scheduler.GetTaskSnapshot("task2"))

	scheduler.Close()
	scheduler.Wait()
}

func TestSchedulerTimeout(t *testing.T) {
	scheduler := NewScheduler(1)

	scheduler.AddTask(
		NewTaskWithTimeout("task1", func(ctx context.Context) (string, error) {
			time.Sleep(2 * time.Second) //
			err := ctx.Err()

			fmt.Println("[][][][]->", err)
			return "task1 result", err
		}, time.Second),
	)

	scheduler.Close()
	scheduler.Wait()

	time.Sleep(2 * time.Second) //

	t.Log(scheduler.GetTaskSnapshot("task1"))
}
