package cron

import (
	"fmt"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/cron"
)

// cronListCmd 列出所有定时任务，包括已禁用的任务。
func cronListCmd(storePath string) {
	cs := cron.NewCronService(storePath, nil)
	jobs := cs.ListJobs(true) // 显示所有任务，包括已禁用的

	if len(jobs) == 0 {
		fmt.Println("No scheduled jobs.")
		return
	}

	fmt.Println("\nScheduled Jobs:")
	fmt.Println("----------------")
	for _, job := range jobs {
		var schedule string
		if job.Schedule.Kind == "every" && job.Schedule.EveryMS != nil {
			schedule = fmt.Sprintf("every %ds", *job.Schedule.EveryMS/1000)
		} else if job.Schedule.Kind == "cron" {
			schedule = job.Schedule.Expr
		} else {
			schedule = "one-time"
		}

		nextRun := "scheduled"
		if job.State.NextRunAtMS != nil {
			nextTime := time.UnixMilli(*job.State.NextRunAtMS)
			nextRun = nextTime.Format("2006-01-02 15:04")
		}

		status := "enabled"
		if !job.Enabled {
			status = "disabled"
		}

		fmt.Printf("  %s (%s)\n", job.Name, job.ID)
		fmt.Printf("    Schedule: %s\n", schedule)
		fmt.Printf("    Status: %s\n", status)
		fmt.Printf("    Next run: %s\n", nextRun)
	}
}

// cronRemoveCmd 根据任务 ID 删除定时任务。
func cronRemoveCmd(storePath, jobID string) {
	cs := cron.NewCronService(storePath, nil)
	if cs.RemoveJob(jobID) {
		fmt.Printf("✓ Removed job %s\n", jobID)
	} else {
		fmt.Printf("✗ Job %s not found\n", jobID)
	}
}

// cronSetJobEnabled 设置指定任务的启用或禁用状态。
func cronSetJobEnabled(storePath, jobID string, enabled bool) {
	cs := cron.NewCronService(storePath, nil)
	job := cs.EnableJob(jobID, enabled)
	if job != nil {
		fmt.Printf("✓ Job '%s' enabled\n", job.Name)
	} else {
		fmt.Printf("✗ Job %s not found\n", jobID)
	}
}
