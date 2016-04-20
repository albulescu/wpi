package main

import (
	"bytes"
)

type WriteJob struct {
	c *connection
	file string
	buffer bytes.Buffer
}

type IndexWorker struct {
	WorkerPool   chan chan WriteJob
	IndexChannel chan WriteJob
	quit         chan bool
}

func NewWorker(pool chan chan WriteJob) IndexWorker {
	return IndexWorker{
		WorkerPool:   pool,
		IndexChannel: make(chan WriteJob),
		quit:         make(chan bool)}
}

func (w IndexWorker) Start() {
	go func() {
		for {
			w.WorkerPool <- w.IndexChannel

			select {
			case job := <-w.IndexChannel:
				writeFile(job.c, job.file, job.buffer)
			case <-w.quit:
				return
			}
		}
	}()
}

func (w IndexWorker) Stop() {
	go func() {
		w.quit <- true
	}()
}

type Dispatcher struct {
	IndexChannel chan WriteJob
	WorkerPool   chan chan WriteJob
	maxWorkers   int
}

func NewDispatcher(ch chan WriteJob, maxWorkers int) *Dispatcher {
	pool := make(chan chan WriteJob, maxWorkers)
	return &Dispatcher{IndexChannel: ch, WorkerPool: pool, maxWorkers: maxWorkers}
}

func (d *Dispatcher) Run() {
	for i := 0; i < d.maxWorkers; i++ {
		worker := NewWorker(d.WorkerPool)
		worker.Start()
	}
	go d.dispatch()
}

func (d *Dispatcher) dispatch() {
	for {
		select {
		case job := <-d.IndexChannel:
			go func(job WriteJob) {
				jobChannel := <-d.WorkerPool
				jobChannel <- job
			}(job)
		}
	}
}
