package scheduler

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"testing/synctest"
	"time"
)

// TestSchedulerCloseReturnsAfterWorkersExit 复现 worker 直接退出后，关闭通知无人接收的问题。
func TestSchedulerCloseReturnsAfterWorkersExit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// 按构造函数初始化，但延后启动 worker，模拟它们尚未获得调度的情况。
		// 不启动构造函数中永不退出的打印协程，以免干扰测试的结束。
		s := &Scheduler{
			workerNum:    2,
			heap:         NewHeap(),
			taskNotice:   make(chan struct{}, 2),
			closeNotice:  make(chan struct{}, 1),
			maxWorkerNum: 2,
		}
		s.workerWaitGroup.Add(2)

		closeReturned := make(chan struct{})
		go func() {
			s.Close() // 会永久阻塞
			close(closeReturned)
		}()

		// 等 Close 阻塞：它已设置 closed，第一次发送填满缓冲，第二次发送等待接收。
		// synctest.Wait 等待其他协程退出或持久阻塞，不依赖 Sleep 猜测执行进度。
		synctest.Wait()

		go s.worker(0)
		go s.worker(1)
		s.Wait()
		synctest.Wait()

		// 两个 worker 都已退出，此时 Close 理应能够返回。
		select {
		case <-closeReturned:
			return
		default:
			t.Errorf("已复现：两个 worker 均已退出，但 Close 仍阻塞，关闭通知缓冲中有 %d 条消息", len(s.closeNotice))
		}

		// 测试主动接收两条通知，释放阻塞的 Close，避免失败后留下协程。
		// 正常使用中没有这位接收者，所以 Close 会一直阻塞。
		<-s.closeNotice
		<-s.closeNotice
		<-closeReturned
	})
}

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
	scheduler := NewScheduler(10 * 2)

	t1 := Task{
		ID:       "t1",
		Priority: 1,
		Func: func(ctx context.Context) (string, error) {
			fmt.Println("t1 executed start")
			time.Sleep(2 * time.Second) //
			fmt.Println("t1 executed end")
			return "t1 result", ctx.Err()
		},
	}

	time.Sleep(2 * time.Second) //
	scheduler.AddTask(t1)

	time.Sleep(2 * time.Second) //

	t2 := Task{
		ID:       "t2",
		Priority: 1,
		Func: func(ctx context.Context) (string, error) {
			fmt.Println("t2 executed start")
			time.Sleep(2 * time.Second) //
			fmt.Println("t2 executed end")
			return "t2 result", ctx.Err()
		},
	}

	scheduler.AddTask(t2)

	fmt.Println("开始添加任务")
	for i := 3; i <= 15; i++ {
		ID := "t" + fmt.Sprintf("%d", i)
		t2 := Task{
			ID:       ID,
			Priority: 1,
			Func: func(ctx context.Context) (string, error) {
				fmt.Printf("t%d executed start\n", i)
				time.Sleep(1 * time.Second) //
				fmt.Printf("t%d executed end\n", i)
				return ID + "Result", ctx.Err()
			},
		}

		scheduler.AddTask(t2)
	}

	fmt.Println("任务添加完成")

	// scheduler.Close()
	scheduler.Wait()

	// time.Sleep(10 * time.Second) //
}
