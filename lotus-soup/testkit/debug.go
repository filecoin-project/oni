package testkit

import (
	"fmt"
	"runtime/pprof"
	"time"
)

func SchedulePeriodicHeapDumps(t *TestEnvironment) {
	if !t.IsParamSet("heap_dump_interval") {
		t.RecordMessage("no heap_dump_interval param, not recording heap dumps")
		return
	}
	interval := t.DurationParam("heap_dump_interval")

	go func() {
		for i := 0; ; i++ {
			f, err := t.CreateRawAsset(fmt.Sprintf("heap-dump-%d.out", i))
			if err != nil {
				t.RecordMessage("unable to create heap dump file: %s", err)
				return
			}
			err = pprof.Lookup("heap").WriteTo(f, 0)
			if err != nil {
				t.RecordMessage("failed to write heap dump: %s", err)
				return
			}
			_ = f.Close()
			time.Sleep(interval)
		}
	}()
}