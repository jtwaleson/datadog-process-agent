package checks

import (
	"os/user"
	"runtime"
	"strconv"
	"time"

	"github.com/DataDog/gopsutil/process"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-process-agent/config"
	"github.com/DataDog/datadog-process-agent/model"
	"github.com/DataDog/datadog-process-agent/util/docker"
)

const (
	// cpuDelta is the amount of time spent between CPU timing checks.
	cpuDelta = 1 * time.Second
)

func CollectProcesses(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error) {
	start := time.Now()
	var err error
	fps, err := process.AllProcesses(cpuDelta, cfg.Concurrency)
	if err != nil {
		return nil, err
	}

	pids := make([]int32, 0, len(fps))
	for _, fp := range fps {
		pids = append(pids, fp.Pid)
	}
	containerByPID, err := docker.ContainersByPID(pids)
	if err != nil && err != docker.ErrDockerNotAvailable {
		log.Warnf("unable to get docker stats: %s", err)
	}

	info, err := collectSystemInfo(cfg)
	if err != nil {
		return nil, err
	}

	groupSize := len(fps) / cfg.ProcLimit
	if len(fps) != cfg.ProcLimit {
		groupSize++
	}
	messages := make([]model.MessageBody, 0, groupSize)
	procs := make([]*model.Process, 0, cfg.ProcLimit)
	for _, fp := range fps {
		if len(fp.Cmdline) == 0 {
			continue
		}
		if config.IsBlacklisted(fp.Cmdline, cfg.Blacklist) {
			continue
		}
		container, _ := containerByPID[fp.Pid]

		if len(procs) >= cfg.ProcLimit {
			messages = append(messages, &model.CollectorProc{
				HostName:  cfg.HostName,
				Processes: procs,
				Info:      info,
				GroupId:   groupID,
				GroupSize: int32(groupSize),
			})
			procs = make([]*model.Process, 0, cfg.ProcLimit)
		}

		procs = append(procs, &model.Process{
			Pid:         fp.Pid,
			Command:     formatCommand(fp),
			User:        formatUser(fp),
			Memory:      formatMemory(fp),
			Cpu:         formatCPU(fp),
			CreateTime:  fp.CreateTime,
			Container:   formatContainer(container),
			OpenFdCount: fp.OpenFdCount,
		})
	}

	messages = append(messages, &model.CollectorProc{
		HostName:  cfg.HostName,
		Processes: procs,
		Info:      info,
		GroupId:   groupID,
		GroupSize: int32(groupSize),
		// FIXME: We should not send this in every payload. Long-term the container
		// ID should be enough context to resolve this metadata on the backend.
		Kubernetes: GetKubernetesMeta(),
	})

	log.Infof("collected processes in %s", time.Now().Sub(start))
	return messages, nil
}

func formatCommand(fp *process.FilledProcess) *model.Command {
	return &model.Command{
		Args:   fp.Cmdline,
		State:  fp.Status,
		Cwd:    fp.Cwd,
		Root:   "",    // TODO
		OnDisk: false, // TODO
		Ppid:   fp.Ppid,
		Pgroup: fp.Pgrp,
		Exe:    fp.Exe,
	}
}

func formatUser(fp *process.FilledProcess) *model.ProcessUser {
	var username string
	var uid, gid int32
	if len(fp.Uids) > 0 {
		u, err := user.LookupId(strconv.Itoa(int(fp.Uids[0])))
		if err == nil {
			username = u.Username
		}
		uid = int32(fp.Uids[0])
	}
	if len(fp.Gids) > 0 {
		gid = int32(fp.Gids[0])
	}

	return &model.ProcessUser{
		Name: username,
		Uid:  uid,
		Gid:  gid,
	}
}

func formatMemory(fp *process.FilledProcess) *model.MemoryStat {
	ms := &model.MemoryStat{
		Rss:  fp.MemInfo.RSS,
		Vms:  fp.MemInfo.VMS,
		Swap: fp.MemInfo.Swap,
	}

	if fp.MemInfoEx != nil {
		ms.Shared = fp.MemInfoEx.Shared
		ms.Text = fp.MemInfoEx.Text
		ms.Lib = fp.MemInfoEx.Lib
		ms.Data = fp.MemInfoEx.Data
		ms.Dirty = fp.MemInfoEx.Dirty
	}
	return ms
}

func formatCPU(fp *process.FilledProcess) *model.CPUStat {
	numCPU := float64(runtime.NumCPU())
	t1, t2 := fp.CpuTime1, fp.CpuTime2
	delta := float64(t2.Timestamp-t1.Timestamp) * float64(numCPU)
	return &model.CPUStat{
		LastCpu:    t2.CPU,
		TotalPct:   calculatePercent(t1.Total(), t2.Total(), delta, numCPU),
		UserPct:    calculatePercent(t1.User, t2.User, delta, numCPU),
		SystemPct:  calculatePercent(t1.System, t2.System, delta, numCPU),
		NumThreads: fp.NumThreads,
		Cpus:       []*model.SingleCPUStat{},
		Nice:       fp.Nice,
		UserTime:   int64(t2.User),
		SystemTime: int64(t2.System),
	}
}

func formatContainer(container *docker.Container) *model.Container {
	// Container will be nill if the process has no container.
	if container == nil {
		return nil
	}
	return &model.Container{
		Type:        container.Type,
		Name:        container.Name,
		Id:          container.ID,
		Image:       container.Image,
		CpuLimit:    float32(container.CPULimit),
		MemoryLimit: container.MemLimit,
	}
}

func calculatePercent(v1, v2, delta, numCPU float64) float32 {
	if delta == 0 {
		return 0
	}
	deltaProc := v2 - v1
	return float32(((deltaProc / delta) * 100) * float64(numCPU))
}