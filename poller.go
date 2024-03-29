package log

import (
	"fmt"
	"sync/atomic"
	"time"
)

func poller() {
	atomic.SwapUint32(&gLogger.look, coreRunning)
	if err := gLogger.loadCurLogFile(); err != nil {
		fmt.Println("----------",err)
		if err = gLogger.createFile(); err != nil {
			panic(err)
		}
	}
	go gLogger.signalHandler()
	ticker := time.NewTicker(time.Millisecond * time.Duration(gLogger.ringInterval))
	now := time.Now()
	next := now.Add(time.Hour * 24)
	next = time.Date(
		next.Year(),
		next.Month(),
		next.Day(),
		0, 0, 0, 0,
		next.Location())
DONE:
	for {
		select {
		case <-gLogger.closeSignal:
			ticker.Stop()
			break DONE
		case <-ticker.C:
			if gLogger.fileWriter.Buffered() > 0 {
				gLogger.sync()
			}
		case n := <-gLogger.Bucket:
			gLogger.fileWriter.Write(n.Bytes())
			gLogger.fileActualSize += n.Len()
			if gLogger.rotate() {
				gLogger.fileWriter.Reset(gLogger.file)
			}
			gLogger.release(n)
		}
	}
}
