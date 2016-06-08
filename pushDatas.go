package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/cadvisor/client"
	"github.com/google/cadvisor/info/v1"
)

func getTag() string {
	//FIXMI:some other message for container
	return ""
}

func pushIt(value float64, timestamp, metric, tags, containerId, counterType, endpoint string) error {
	valueRet := strconv.FormatFloat(value, 'f', -1, 64)
	postThing := `[{"metric": "` + metric + `", "endpoint": "` + endpoint + `", "timestamp": ` + timestamp + `,"step": ` + "60" + `,"value": ` + valueRet + `,"counterType": "` + counterType + `","tags": "` + tags + `"}]`
	fmt.Printf("%s: %s \n", metric, postThing)
	LogRun(postThing)
	//push data to falcon-agent
	url := "http://127.0.0.1:1988/v1/push"
	resp, err := http.Post(url, "application/x-www-form-urlencoded", strings.NewReader(postThing))
	if err != nil {
		LogErr(err, "Post err in pushIt")
		return err
	}
	defer resp.Body.Close()
	_, err1 := ioutil.ReadAll(resp.Body)
	if err1 != nil {
		LogErr(err1, "ReadAll err in pushIt")
		return err1
	}
	return nil
}

func pushData() error {

	var numStats int = 60
	client, err := client.NewClient("http://localhost:8080/")
	if err != nil {
		LogErr(err, "Can not connect the localhost:8080")
		return err
	}

	mInfo, err := client.MachineInfo()
	if err != nil {
		LogErr(err, "Can not get machine info")
		return err
	}

	cpuNum := int(mInfo.NumCores)

	request := v1.ContainerInfoRequest{NumStats: numStats} //get the cadvisor data
	cAllInfo, err := client.AllDockerContainers(&request)

	if err != nil {
		fmt.Println(err)
	}

	t := time.Now().Unix()
	timestamp := fmt.Sprintf("%d", t)

	for i := 0; i < len(cAllInfo); i++ {
		var (
			cpuUsageTotal  uint64
			cpuUsageUser   uint64
			cpuUsageSys    uint64
			memoryUsage    uint64
			diskIORead     uint64
			diskIOWrite    uint64
			networkRxBytes uint64
			networkTxBytes uint64
		)
		cpuUsageTotal = 0
		cpuUsageUser = 0
		cpuUsageSys = 0
		memoryUsage = 0
		diskIORead = 0
		diskIOWrite = 0
		networkRxBytes = 0
		networkTxBytes = 0

		cInfo := cAllInfo[i]

		containerId := cInfo.Id
		endpoint := cInfo.Id[:12]
		memoryTotal := cInfo.Spec.Memory.Limit

		tag := getTag()

		j := len(cInfo.Stats)
		cpuUsageTotal = cInfo.Stats[j-1].Cpu.Usage.Total - cInfo.Stats[0].Cpu.Usage.Total
		cpuUsageUser = cInfo.Stats[j-1].Cpu.Usage.User - cInfo.Stats[0].Cpu.Usage.User
		cpuUsageSys = cInfo.Stats[j-1].Cpu.Usage.System - cInfo.Stats[0].Cpu.Usage.System
		networkRxBytes = cInfo.Stats[j-1].Network.InterfaceStats.RxBytes - cInfo.Stats[0].Network.InterfaceStats.RxBytes
		networkTxBytes = cInfo.Stats[j-1].Network.InterfaceStats.TxBytes - cInfo.Stats[0].Network.InterfaceStats.TxBytes

		for n := 0; n < j; n++ {
			memoryUsage += cInfo.Stats[n].Memory.Usage
			for m := 0; m < len(cInfo.Stats[n].DiskIo.IoServiceBytes); m++ {
				diskIORead += cInfo.Stats[n].DiskIo.IoServiceBytes[m].Stats["Read"]
				diskIOWrite += cInfo.Stats[n].DiskIo.IoServiceBytes[m].Stats["Write"]
			}
		}

		if err := pushCPU(numStats, cpuNum, cpuUsageTotal, cpuUsageUser, cpuUsageSys, timestamp, tag, containerId, endpoint); err != nil {
			LogErr(err, "pushCPU err")
			return err
		}

		if err := pushMem(numStats, memoryTotal, memoryUsage, timestamp, tag, containerId, endpoint); err != nil {
			LogErr(err, "pushMem err")
			return err
		}

		if err := pushDiskIO(numStats, diskIORead, diskIOWrite, timestamp, tag, containerId, endpoint); err != nil {
			LogErr(err, "pushDiskIO err")
			return err
		}

		if err := pushNet(numStats, networkRxBytes, networkTxBytes, timestamp, tag, containerId, endpoint); err != nil {
			LogErr(err, "pushNet err")
			return err
		}
	}
	return nil
}

func pushCPU(numStats, cpuNum int, cpuUsageTotal, cpuUsageUser, cpuUsageSys uint64, timestamp, tags, containerId, endpoint string) error {
	LogRun("pushCPU")

	cpuUsageTotalRet := float64(cpuUsageTotal) / (float64(cpuNum) * 100000000 * float64(numStats)) * 100
	cpuUsageUserRet := float64(cpuUsageUser) / (float64(cpuNum) * 100000000 * float64(numStats)) * 100
	cpuUsageSysRet := float64(cpuUsageSys) / (float64(cpuNum) * 100000000 * float64(numStats)) * 100

	if err := pushIt(cpuUsageTotalRet, timestamp, "cpu.busy", tags, containerId, "GAUGE", endpoint); err != nil {
		LogErr(err, "pushIt err in cpu.busy")
		return err
	}

	if err := pushIt(cpuUsageUserRet, timestamp, "cpu.user", tags, containerId, "GAUGE", endpoint); err != nil {
		LogErr(err, "pushIt err in cpu.user")
		return err
	}

	if err := pushIt(cpuUsageSysRet, timestamp, "cpu.system", tags, containerId, "GAUGE", endpoint); err != nil {
		LogErr(err, "pushIt err in cpu.system")
		return err
	}

	return nil
}

func pushMem(numStats int, memLimit, memoryusage uint64, timestamp, tags, containerId, endpoint string) error {
	LogRun("pushMem")

	memoryUsageRet := memoryusage / uint64(numStats)

	if err := pushIt(float64(memLimit), timestamp, "mem.memtotal", tags, containerId, "GAUGE", endpoint); err != nil {
		LogErr(err, "pushIt err in mem.memtotal")
		return err
	}

	if err := pushIt(float64(memoryUsageRet), timestamp, "mem.memused", tags, containerId, "GAUGE", endpoint); err != nil {
		LogErr(err, "pushIt err in mem.memtotal")
		return err
	}

	memUsedPercent := float64(memoryUsageRet) / float64(memLimit) * 100
	if err := pushIt(memUsedPercent, timestamp, "mem.memused.percent", tags, containerId, "GAUGE", endpoint); err != nil {
		LogErr(err, "pushIt err in mem.memtotal")
		return err
	}

	return nil
}

func pushNet(numStats int, networkRxBytes, networkTxBytes uint64, timestamp, tags, containerId, endpoint string) error {
	LogRun("pushNet")

	networkRxRet := float64(networkRxBytes) / float64(numStats)
	if err := pushIt(networkRxRet, timestamp, "net.if.in.bytes", tags, containerId, "GAUGE", endpoint); err != nil {
		LogErr(err, "pushIt err in net.if.in.bytes")
		return err
	}
	networkTxRet := float64(networkTxBytes) / float64(numStats)
	if err := pushIt(networkTxRet, timestamp, "net.if.out.bytes", tags, containerId, "GAUGE", endpoint); err != nil {
		LogErr(err, "pushIt err in net.if.out.bytes")
		return err
	}
	return nil
}

func pushDiskIO(numStats int, diskIORead, diskIOWrite uint64, timestamp, tags, containerId, endpoint string) error {
	LogRun("pushDiskIo")
	diskIOReadRet := float64(diskIORead) / float64(numStats)
	diskIOWriteRet := float64(diskIOWrite) / float64(numStats)
	if err := pushIt(diskIOReadRet, timestamp, "disk.io.read_bytes", tags, containerId, "COUNTER", endpoint); err != nil {
		LogErr(err, "pushIt err in pushDiskIo")
	}
	if err := pushIt(diskIOWriteRet, timestamp, "disk.io.write_bytes", tags, containerId, "COUNTER", endpoint); err != nil {
		LogErr(err, "pushIt err in pushDiskIo")
	}

	return nil
}
