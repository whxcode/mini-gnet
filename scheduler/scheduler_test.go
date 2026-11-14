package scheduler

import (
	"context"
	"errors"
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

func _TestSchedulerTimeout(t *testing.T) {
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

func _TestSchedulerPriority(t *testing.T) {
	scheduler := NewScheduler(2)

	t1 := Task{
		ID:       "t1",
		Priority: 1,
		Func: func(ctx context.Context) (string, error) {
			time.Sleep(2 * time.Second) //
			fmt.Println("t1 executed")
			return "t1 result", ctx.Err()
		},
	}

	t2 := Task{
		ID:       "t2",
		Priority: 2,
		Func: func(ctx context.Context) (string, error) {
			time.Sleep(2 * time.Second) //
			fmt.Println("t2 executed")
			return "t2 result", ctx.Err()
		},
	}

	t3 := Task{
		ID:       "t3",
		Priority: 3,
		Func: func(ctx context.Context) (string, error) {
			time.Sleep(2 * time.Second) //
			fmt.Println("t3 executed")
			return "t3 result", ctx.Err()
		},
	}

	scheduler.AddTask(t1, t2, t3)

	scheduler.Close()
	scheduler.Wait()

	time.Sleep(2 * time.Second) //
}

func _TestSchedulerClose(t *testing.T) {
	scheduler := NewScheduler(2)

	t1 := Task{
		ID:       "t1",
		Priority: 1,
		Func: func(ctx context.Context) (string, error) {
			time.Sleep(2 * time.Second) //
			fmt.Println("t1 executed")
			return "t1 result", ctx.Err()
		},
	}

	t2 := Task{
		ID:       "t2",
		Priority: 2,
		Func: func(ctx context.Context) (string, error) {
			time.Sleep(2 * time.Second) //
			fmt.Println("t2 executed")
			return "t2 result", ctx.Err()
		},
	}

	t3 := Task{
		ID:       "t3",
		Priority: 3,
		Func: func(ctx context.Context) (string, error) {
			time.Sleep(2 * time.Second) //
			fmt.Println("t3 executed")
			return "t3 result", ctx.Err()
		},
	}

	scheduler.AddTask(t1)
	scheduler.Close()

	if !errors.Is(scheduler.AddTask(t2, t3), ErrClosed) {
		t.Errorf("Expected error when adding tasks after scheduler is closed, but got nil")
	}

	scheduler.Close()
	scheduler.Wait()

	time.Sleep(2 * time.Second) //
}

func TestSchedulerShirk(t *testing.T) {
	scheduler := NewScheduler(2)

	t1 := Task{
		ID:       "t1",
		Priority: 1,
		Func: func(ctx context.Context) (string, error) {
			fmt.Println("t1 executed start")
			time.Sleep(4 * time.Second) //
			fmt.Println("t1 executed end")
			return "t1 result", ctx.Err()
		},
	}

	time.Sleep(2 * time.Second) //
	scheduler.AddTask(t1)
	scheduler.Close()
	scheduler.Wait()

	// time.Sleep(10 * time.Second) //
}
