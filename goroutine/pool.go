package routine

import (
	"fmt"
	"sync"
	"time"
)

// 单生产者,多消费者模式
// 线程次
func workerPool() {
	const numWorkers = 100
	const numTasks = 100000

	tasks := make(chan int, numTasks)

	var wg sync.WaitGroup

	for i := range numWorkers {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for task := range tasks {
				processTask(workerID, task)
				// Process the task (in this case, just print it)
				// fmt.Printf("Worker %d processing task %d\n", workerID, task)
			}
		}(i)
	}

	for i := range numTasks {
		tasks <- i
	}

	close(tasks)

	wg.Wait()
}

func processTask(workerID, task int) {
	fmt.Printf("Worker %d processing task %d\n", workerID, task)
	time.Sleep(10 * time.Millisecond) // Simulate some work
}

func Test01() {
	workerPool()
}
