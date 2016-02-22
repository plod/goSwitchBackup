package main

import (
	"flag"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"log"
	"strings"
	"time"
)

var ip = flag.String("ip", "", "location of the switch to manage")
var userName = flag.String("userName", "", "username to connect to switch")
var normalPw = flag.String("normalPW", "", "the standard switch ssh password")
var enablePw = flag.String("enablePW", "", "the enable password for esculated priv")
var tftpServer = flag.String("tftpServer", "", "the tftp server ip address")

func readBuffForString(whattoexpect string, sshOut io.Reader, buffRead chan<- string) {
	buf := make([]byte, 1000)
	n, err := sshOut.Read(buf) //this reads the ssh terminal
	waitingString := ""
	if err == nil {
		waitingString = string(buf[:n])
	}
	for (err == nil) && (!strings.Contains(waitingString, whattoexpect)) {
		n, err = sshOut.Read(buf)
		waitingString += string(buf[:n])
		//fmt.Println(waitingString) //uncommenting this might help you debug if you are coming into errors with timeouts when correct details entered

	}
	buffRead <- waitingString
}
func readBuff(whattoexpect string, sshOut io.Reader, timeoutSeconds int) string {
	ch := make(chan string)
	go func(whattoexpect string, sshOut io.Reader) {
		buffRead := make(chan string)
		go readBuffForString(whattoexpect, sshOut, buffRead)
		select {
		case ret := <-buffRead:
			ch <- ret
		case <-time.After(time.Duration(timeoutSeconds) * time.Second):
			handleError(fmt.Errorf("%d", timeoutSeconds), true, "Waiting for \""+whattoexpect+"\" took longer than %s seconds, perhaps you've entered incorrect details?")
		}
	}(whattoexpect, sshOut)
	return <-ch
}
func writeBuff(command string, sshIn io.WriteCloser) (int, error) {
	returnCode, err := sshIn.Write([]byte(command + "\r"))
	return returnCode, err
}
func handleError(e error, fatal bool, customMessage ...string) {
	var errorMessage string
	if e != nil {
		if len(customMessage) > 0 {
			errorMessage = strings.Join(customMessage, " ")
		} else {
			errorMessage = "%s"
		}
		if fatal == true {
			log.Fatalf(errorMessage, e)
		} else {
			log.Print(errorMessage, e)
		}
	}
}
func main() {
	flag.Parse()
	/*
		  fmt.Println("IP Chosen: ", *ip)
			fmt.Println("Username", *userName)
			fmt.Println("Normal PW", *normalPw)
			fmt.Println("Enable PW", *enablePw)
			fmt.Println("TFTP Server", *tftpServer)
	*/
	sshConfig := &ssh.ClientConfig{
		User: *userName,
		Auth: []ssh.AuthMethod{
			ssh.Password(*normalPw),
		},
	}
	sshConfig.Config.Ciphers = append(sshConfig.Config.Ciphers, "aes128-cbc")
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	connection, err := ssh.Dial("tcp", *ip+":22", sshConfig)
	if err != nil {
		log.Fatalf("Failed to dial: %s", err)
	}
	session, err := connection.NewSession()
	handleError(err, true, "Failed to create session: %s")
	sshOut, err := session.StdoutPipe()
	handleError(err, true, "Unable to setup stdin for session: %v")
	sshIn, err := session.StdinPipe()
	handleError(err, true, "Unable to setup stdout for session: %v")
	if err := session.RequestPty("xterm", 0, 200, modes); err != nil {
		session.Close()
		handleError(err, true, "request for pseudo terminal failed: %s")
	}
	if err := session.Shell(); err != nil {
		session.Close()
		handleError(err, true, "request for shell failed: %s")
	}
	readBuff(">", sshOut, 2)
	if _, err := writeBuff("enable", sshIn); err != nil {
		handleError(err, true, "Failed to run: %s")
	}
	if _, err := writeBuff(*enablePw, sshIn); err != nil {
		handleError(err, true, "Failed to run: %s")
	}
	readBuff("#", sshOut, 2)
	if _, err := writeBuff("copy running-config tftp", sshIn); err != nil {
		handleError(err, true, "Failed to run: %s")
	}
	readBuff("Address or name of remote host []?", sshOut, 2)
	if _, err := writeBuff(*tftpServer, sshIn); err != nil {
		handleError(err, true, "Failed to run: %s")
	}
	readBuff("confg]?", sshOut, 2)
	filename := []string{"switchBackup-", strings.Replace(*ip, ".", "-", -1), "_", strings.Replace(time.Now().Format(time.RFC3339), ":", "", -1)}
	if _, err := writeBuff(strings.Join(filename, ""), sshIn); err != nil {
		handleError(err, true, "Failed to run: %s")
	}
	fmt.Println(readBuff("bytes/sec)", sshOut, 60))
	session.Close()
}
