package routine

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

func contextCancellation() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

	defer cancel()

	var wg sync.WaitGroup

	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			if err := doWoker(ctx, id); err != nil {
				fmt.Println("任务取消:", id, err)
			}
		}(i)
	}

	wg.Wait()

	fmt.Println("所有任务完成")
}

func doWoker(ctx context.Context, id int) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			fmt.Println("模拟工作中:", id)
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func Test03() {
	type G struct {
		ctx context.Context
	}
}

func Test04() {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				fmt.Println("任务取消")

				return
			default:
				fmt.Println("模拟工作中")
				time.Sleep(2 * time.Second)
			}
		}
	}()

	time.Sleep(time.Second)

	cancel()
	wg.Wait()
	fmt.Println("所有任务完成")
}

func handler(w http.ResponseWriter, r *http.Request) {
	// 1. 创建一个 2 秒后自动取消的 context。
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// 4. handler 返回时执行清理。
	defer fmt.Println("handler 结束，执行清理")

	fmt.Println("开始处理请求")

	select {
	case <-time.After(5 * time.Second):
		// 模拟工作完成。
		fmt.Fprintln(w, "处理成功")

	case <-ctx.Done():
		// 3. 收到取消通知，返回超时响应。
		fmt.Println("停止处理：", ctx.Err())
		http.Error(w, "请求超时", http.StatusGatewayTimeout)

		return
	}
}

func test01() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

	defer func() {
		fmt.Println("取消 context")
		cancel()
	}()

	go func() {
		for {
			if err := ctx.Err(); err != nil {
				fmt.Println("任务取消:", err)

				return
			}

			time.Sleep(500 * time.Millisecond)
		}
	}()

	time.Sleep(10 * time.Second)
}

func Test05() {
	test01()
}
