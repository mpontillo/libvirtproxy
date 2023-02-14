package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
)

var UnableToGetProcessName = errors.New("unable to get process name")

type processInfo struct {
	executable string
	args       []string
}

/*
func processNameByPID(pid int32) (string, error) {
	exeLink := fmt.Sprintf("/proc/%d/exe", pid)
	exeName, err := os.Readlink(exeLink)
	if err != nil {
		b := make([]byte, 1024)
		// fall back to /proc/<pid>/procCmdlineFile
		procCmdlineFilename := fmt.Sprintf("/proc/%d/cmdline", pid)
		procCmdlineFile, err := os.Open(procCmdlineFilename)
		if err != nil {
			return "", fmt.Errorf("unable to open: %s", procCmdlineFilename)
		}
		n, err := procCmdlineFile.Read(b)
		if n > 0 && (err != nil || err != io.EOF) {
			sep := []byte{0}
			commands := bytes.Split(b, sep)
			return string(commands[0]), err
		}
		return "", UnableToGetProcessName
	}
	return exeName, nil
} */

func getProcessArguments(pid int32) []string {
	procCmdlineFilename := fmt.Sprintf("/proc/%d/cmdline", pid)
	procCmdlineFile, err := os.Open(procCmdlineFilename)
	if err != nil {
		return nil
	}
	b, err := io.ReadAll(procCmdlineFile)
	if err != nil {
		return nil
	}
	sep := []byte{0}
	commands := bytes.Split(b, sep)
	result := make([]string, len(commands), len(commands))
	for i := 0; i < len(commands); i++ {
		result[i] = string(commands[i])
	}
	return result
}

func getProcessInfo(pid int32) processInfo {
	var info processInfo
	exeLink := fmt.Sprintf("/proc/%d/exe", pid)
	exeName, _ := os.Readlink(exeLink)
	info.executable = exeName
	info.args = getProcessArguments(pid)
	return info
}

func accept(client *net.UnixConn) {
	defer client.Close()
	f, err := client.File()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot get file for client: %v\n", err)
		return
	}
	cred, err := syscall.GetsockoptUcred(int(f.Fd()), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot get peer credential for client: %v\n", err)
		return
	}
	info := getProcessInfo(cred.Pid)
	fmt.Fprintf(os.Stderr, "credential: %+v\ninfo: %+v\n", cred, info)

	proxy, err := net.Dial("unix", "/run/libvirt/libvirt-sock")
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to create proxy to libvirt: %v\n", err)
		return
	}
	defer proxy.Close()

	requestBytesChannel := make(chan int64)
	responseBytesChannel := make(chan int64)

	// copy data from the client to the real socket
	go func() {
		requestSize, err := io.Copy(proxy, client)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error proxying request bytes: %v\n", err)
		}
		requestBytesChannel <- requestSize
	}()

	// copy responses from the real socket back to the client
	go func() {
		responseSize, err := io.Copy(client, proxy)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error proxying response bytes: %v\n", err)
		}
		responseBytesChannel <- responseSize
	}()

	// the .Close() calls are deferred; wait to return until they complete.
	requestSize := <-requestBytesChannel
	fmt.Printf("proxied %d request bytes\n", requestSize)

	responseSize := <-responseBytesChannel
	fmt.Printf("proxied %d response bytes\n", responseSize)
}

func main() {
	args := os.Args[1:]
	var binaryName = "libvirtproxy"

	// validate arguments
	if len(args) != 1 {
		exe, err := os.Executable()
		if err == nil {
			binaryName = exe
		}
		fmt.Fprintf(os.Stderr, "usage: %s <socket-file>\n", binaryName)
		os.Exit(1)
	}
	listenSocket := args[0]

	listener, err := net.ListenUnix("unix", &net.UnixAddr{
		Name: listenSocket,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to open listener for socket '%s': %v\n", listenSocket, err)
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "%s: listening on '%s'\n", binaryName, listenSocket)
	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to accept connection: %v\n", err)
		}
		go accept(conn)
	}
}
