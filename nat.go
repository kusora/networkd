package main

import (
	"bytes"
	"fmt"
	"github.com/Sirupsen/logrus"
	"os/exec"
	"strings"
	"time"
)

// TODO(xutao) support more interfaces
type ItemAcl struct {
	srcIp   string
	srcPort string
	dstIp   string
	dstPort string
	proto   string
	chain   string
	device  string
	action  string
	comment string
}

const IptablesNotFound = "iptables: No chain/target/match by that name."
const IptablesLocked = "Another app is currently holding the xtables lock."
const IptablesChainFound = "iptables: Chain already exists."
const CalicoRuleExist = "Rule already present, skipping."
const CalicoProfileNotExist = "not found."

func doArp(ip string, netInterface string) bool {
	// arping -c 4 -w 3 -U -I eth0 10.106.170.202
	cmdName := "arping"
	cmdArgs := []string{
		"-c", "4", "-w", "3", "-U", "-I", netInterface, ip,
	}
	out, err := doCmd(cmdName, cmdArgs)
	if err != nil {
		log.Error(out.String())
		log.Fatal(err)
	}
	return true
}

func doPing(ip string) bool {
	// ping -n -q -c 3 10.106.170.202
	cmdName := "ping"
	cmdArgs := []string{
		"-n", "-q", "-c", "3", ip,
	}
	_, err := doCmd(cmdName, cmdArgs)
	if err != nil {
		return false
	}
	return true
}

// TODO(xutao) more detailed check: interface
func doIpAddrCheck(ip string, netInterface string) bool {
	cmdName := "ip"
	cmdArgs := []string{"-o", "-4", "addr", "show", "dev", netInterface}
	out, err := doCmd(cmdName, cmdArgs)
	if err != nil {
		log.Error(out.String())
		log.Fatal(err)
	}
	if !strings.Contains(out.String(), fmt.Sprintf("%s/32", ip)) {
		return false
	}
	return true
}

func doIpAddrAdd(ip string, netInterface string) bool {
	cmdName := "ip"
	cmdArgs := []string{"-o", "addr", "add", ip + "/32", "dev", netInterface}
	log.Debug(fmt.Sprintf("%v %v", cmdName, cmdArgs))
	out, err := doCmd(cmdName, cmdArgs)
	if err != nil {
		log.Error(out.String())
		log.Fatal(err)
	}
	return true
}

func doIpAddrDelete(ip string, netInterface string) bool {
	cmdName := "ip"
	cmdArgs := []string{"-o", "addr", "delete", ip + "/32", "dev", netInterface}
	log.Debug(fmt.Sprintf("%v %v", cmdName, cmdArgs))
	out, err := doCmd(cmdName, cmdArgs)
	if err != nil {
		log.Error(out.String())
		log.Fatal(err)
	}
	return true
}

func initIptables() {
	// check lain-PREROUTING
	// add lain-PREROUTING
	// check lain-PREROUTING rule
	// add lain-PREROUTING rule
	initIptablesChain("lain-PREROUTING")
	// check lain-OUTPUT
	// add lain-OUTPUT
	// check lain-OUTPUT rule
	// add lain-OUTPUT rule
	initIptablesChain("lain-OUTPUT")
}

func initIptablesChain(name string) {
	var cmdArgs []string
	var message string
	tempArray := strings.Split(name, "-")
	filterName := tempArray[len(tempArray)-1]
	cmdName := "iptables"
	cmdArgs = []string{
		"-n",
		"-t", "nat",
		"-L", name,
	}
	out, err := doCmd(cmdName, cmdArgs)
	if err != nil {
		message = out.String()
		if !strings.Contains(message, IptablesNotFound) {
			log.Print(message)
			log.Fatal(err)
		}
		cmdArgs = []string{
			"-t", "nat",
			"-N", name,
		}
		out, err = doCmd(cmdName, cmdArgs)
		if err != nil {
			message = out.String()
			if !strings.Contains(message, IptablesChainFound) {
				log.Print(message)
				log.Fatal(err)
			}
		}
	}
	cmdArgs = []string{
		"-t", "nat",
		"-C", filterName,
		"-j", name,
	}
	out, err = doCmd(cmdName, cmdArgs)
	if err != nil {
		message := out.String()
		if !strings.Contains(message, IptablesNotFound) {
			log.Print(message)
			log.Fatal(err)
		}
		cmdArgs = []string{
			"-t", "nat",
			"-A", filterName,
			"-j", name,
		}
		doCmd(cmdName, cmdArgs)
	}
}

func doIptablesCheck(ip string, port string, containerIp string, containerPort string, proto string, comment string) bool {
	cmdName := "iptables"
	cmdArgs := []string{
		"-t", "nat",
		"-C", "lain-PREROUTING",
		"-p", proto,
		"-d", ip,
		"--dport", port,
		"-j", "DNAT",
		"--to-destination", containerIp + ":" + containerPort,
		"-m", "comment",
		"--comment", comment, // AppName.ProcName
	}
	out, err := doCmd(cmdName, cmdArgs)
	if err == nil {
		return true
	}
	message := out.String()
	if !strings.Contains(message, IptablesNotFound) {
		log.Print(message)
		log.Fatal(err)
	}
	return false
}

func doIptablesClean(cmd string) bool {
	cmdName := "iptables"
	cmd = strings.TrimSuffix(cmd, "\n")
	cmd = strings.Replace(cmd, "\"", "", -1)
	cmdArgs := strings.Split(cmd, " ")
	cmdArgs = append([]string{"-t", "nat", "-D"}, cmdArgs[1:]...)
	out, err := doCmd(cmdName, cmdArgs)
	if err != nil {
		log.Error(out.String())
		log.Fatal(err)
	}
	return true
}

func cleanIptables(ip string) bool {
	line, err := doIptablesList("lain-PREROUTING")
	if err != nil {
		log.Error(err)
		return false
	}
	for _, value := range line {
		if strings.Contains(value, fmt.Sprintf("%s/32", ip)) {
			doIptablesClean(value)
		}
	}
	line, err = doIptablesList("lain-OUTPUT")
	if err != nil {
		log.Error(err)
		return false
	}
	for _, value := range line {
		if strings.Contains(value, fmt.Sprintf("%s/32", ip)) {
			doIptablesClean(value)
		}
	}
	return true
}

func isInIptables(ip string) bool {
	line, err := doIptablesList("lain-PREROUTING")
	if err != nil {
		log.Error(err)
		return true
	}
	for _, value := range line {
		if strings.Contains(value, fmt.Sprintf("%s/32", ip)) {
			return true
		}
	}
	line, err = doIptablesList("lain-OUTPUT")
	if err != nil {
		log.Error(err)
		return true
	}
	for _, value := range line {
		if strings.Contains(value, fmt.Sprintf("%s/32", ip)) {
			return true
		}
	}
	return false
}

func doIptablesAdd(ip string, port string, containerIp string, containerPort string, proto string, comment string) bool {
	cmdName := "iptables"
	cmdArgs := []string{
		"-t", "nat",
		"-A", "lain-PREROUTING",
		"-p", proto,
		"-d", ip,
		"--dport", port,
		"-j", "DNAT",
		"--to-destination", containerIp + ":" + containerPort,
		"-m", "comment",
		"--comment", comment, // AppName.ProcName
	}
	out, err := doCmd(cmdName, cmdArgs)
	if err != nil {
		log.Error(out.String())
		log.Fatal(err)
	}
	return true
}

func doIptablesDelete(ip string, port string, containerIp string, containerPort string, proto string, comment string) bool {
	cmdName := "iptables"
	cmdArgs := []string{
		"-t", "nat",
		"-D", "lain-PREROUTING",
		"-p", proto,
		"-d", ip,
		"--dport", port,
		"-j", "DNAT",
		"--to-destination", containerIp + ":" + containerPort,
		"-m", "comment",
		"--comment", comment, // AppName.ProcName
	}
	out, err := doCmd(cmdName, cmdArgs)
	if err != nil {
		log.Error(out.String())
		log.Fatal(err)
	}
	return true
}

func doIptablesAddAcl(item *ItemAcl) bool {
	ok := doIptablesCheckAcl(item)
	if ok {
		return true
	}
	ok = doIptablesAcl(item)
	if !ok {
		return false
	}
	return true
}

func doIptablesDeleteAcl(item *ItemAcl) bool {
	ok := doIptablesCheckAcl(item)
	if !ok {
		return true
	}
	ok = doIptablesAcl(item)
	if !ok {
		return false
	}
	return true
}

func doIptablesCheckAcl(item *ItemAcl) bool {
	cmdName := "iptables"
	cmdArgs := []string{
		"-t", "nat",
		"-C", item.chain,
		"-p", item.proto,
		"-d", item.srcIp,
		"--dport", item.srcPort,
		"-j", "DNAT",
		"--to-destination", item.dstIp + ":" + item.dstPort,
		"-m", "comment",
		"--comment", item.comment, // AppName.ProcName
	}
	if item.device != "" {
		cmdArgs = append(cmdArgs, []string{"-o", item.device}...)
	}

	var message string
	var err error
	for i := 1; i <= 3; i++ {
		out, err := doCmd(cmdName, cmdArgs)
		if err == nil {
			return true
		}
		message = out.String()
		if strings.Contains(message, IptablesNotFound) {
			return false
		}
		if strings.Contains(message, IptablesLocked) {
			time.Sleep(15 * time.Second)
			continue
		}
		log.Print(message)
		log.Fatal(err)
	}
	if err != nil {
		log.Print(message)
		log.Fatal(err)
	}
	// fake return
	return true
}

func doIptablesAcl(item *ItemAcl) bool {
	cmdName := "iptables"
	cmdArgs := []string{
		"-t", "nat",
		item.action, item.chain,
		"-p", item.proto,
		"-d", item.srcIp,
		"--dport", item.srcPort,
		"-j", "DNAT",
		"--to-destination", item.dstIp + ":" + item.dstPort,
		"-m", "comment",
		"--comment", item.comment, // AppName.ProcName
	}
	if item.device != "" {
		cmdArgs = append(cmdArgs, []string{"-o", item.device}...)
	}
	out, err := doCmd(cmdName, cmdArgs)
	if err != nil {
		log.Error(out.String())
		log.Fatal(err)
	}
	return true
}

func doIptablesList(chain string) (l []string, err error) {
	cmdName := "iptables"
	cmdArgs := []string{
		"-t", "nat",
		"-S", chain,
	}
	var lines []string
	out, err := doCmd(cmdName, cmdArgs)
	if err != nil {
		return nil, err
	}
	for {
		line, err := out.ReadString('\n')
		if err != nil {
			break
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func doCalicoAddProfile(name string) bool {
	// calicoctl profile add NAME
	cmdName := "calicoctl"
	cmdArgs := []string{"profile", "add", name}
	out, err := doCmd(cmdName, cmdArgs)
	if err != nil {
		log.Error(out.String())
		log.Fatal(err)
	}
	// added or existed
	return true
}

func doCalicoAddProfileRule(name string, proto string, port string) bool {
	// calicoctl profile {{ item }} rule add inbound --at=1 allow udp to ports 53
	cmdName := "calicoctl"
	cmdArgs := []string{
		"profile", name, "rule",
		"add", "inbound", "--at=1",
		"allow", proto, "to", "ports", port,
	}
	out, err := doCmd(cmdName, cmdArgs)
	if err == nil {
		// added or existed
		return true
	}
	message := out.String()
	if !strings.Contains(message, CalicoProfileNotExist) {
		log.Print(message)
		log.Fatal(err)
	}
	return false
}

func doCalicoAddProfileDefaultRule(name string) bool {
	// calicoctl profile {{ item  }} rule add inbound --at=1 allow from tag default
	cmdName := "calicoctl"
	cmdArgs := []string{
		"profile", name, "rule",
		"add", "inbound", "--at=1",
		"allow", "from", "tag", "default",
	}
	out, err := doCmd(cmdName, cmdArgs)
	if err == nil {
		// added or existed
		return true
	}
	message := out.String()
	if !strings.Contains(message, CalicoProfileNotExist) {
		log.Print(message)
		log.Fatal(err)
	}
	return false
}

//TODO(xutao) doCalicoDeleteProfile
//TODO(xutao) doCalicoDeleteProfileRule
//TODO(xutao) doCalicoDeleteProfileDefaultRule

func doCmd(cmdName string, cmdArgs []string) (bytes.Buffer, error) {
	var cmdOut bytes.Buffer
	var cmdErr bytes.Buffer
	cmd := exec.Command(cmdName, cmdArgs...)
	cmd.Stdout = &cmdOut
	cmd.Stderr = &cmdErr
	err := cmd.Run()
	if err != nil {
		log.WithFields(logrus.Fields{
			"cmd":  cmdName,
			"args": cmdArgs,
			"err":  err,
		}).Debug("Fail to run cmd")
		return cmdErr, err
	}
	log.WithFields(logrus.Fields{
		"cmd":  cmdName,
		"args": cmdArgs,
	}).Debug("Success to run cmd")
	return cmdOut, err
}
