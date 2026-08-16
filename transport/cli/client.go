package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/peterh/liner"
)

type Client struct {
	name          string
	ctx           context.Context
	address       string
	sequence      uint16
	conn          net.Conn
	liner         *liner.State
	mutex         sync.Mutex
	exitChan      chan struct{}
	readyChan     chan struct{}
	commandChan   chan *Frame
	completerChan chan *Frame
	Timeout       time.Duration
	exitFlag      int32
}

// getSequence returns the next monotonic sequence number, wrapping
// from math.MaxUint16 back to 1 (0 is reserved as "uninitialized").
func (client *Client) getSequence() uint16 {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	client.sequence++
	if client.sequence == 0 { // wrapped past math.MaxUint16
		client.sequence = 1
	}
	return client.sequence
}

func (client *Client) dialContext(ctx context.Context, address string) (conn net.Conn, err error) {
	var (
		pos     int
		network string
		dialer  net.Dialer
	)
	if pos = strings.Index(address, "://"); pos > -1 {
		network = address[:pos]
		address = address[pos+3:]
	} else {
		network = "tcp"
	}
	if conn, err = dialer.DialContext(ctx, network, address); err != nil {
		return
	}
	return
}

func (client *Client) renderBanner(info *handshake) {
	client.name = info.Name
	fmt.Printf("Welcome to the %s(%s) monitor\n", info.Name, info.Version)
	fmt.Printf("Your connection id is %d\n", info.ID)
	fmt.Printf("Last login: %s from %s\n", info.ServerTime.Format(time.RFC822), info.RemoteAddr)
	fmt.Printf("Type 'help' for help. Type 'exit' for quit. Type 'cls' to clear input statement.\n")
}

func (client *Client) ioLoop(r io.Reader) {
	defer func() {
		_ = client.Close()
	}()
	for {
		frame, err := readFrame(r)
		if err != nil {
			return
		}
		switch frame.Type {
		case PacketTypeHandshake:
			info := &handshake{}
			if err = json.Unmarshal(frame.Data, info); err == nil {
				client.renderBanner(info)
			}
			select {
			case client.readyChan <- struct{}{}:
			case <-client.exitChan:
				return
			}
		case PacketTypeCompleter:
			select {
			case client.completerChan <- frame:
			case <-client.exitChan:
				return
			}
		case PacketTypeCommand:
			select {
			case client.commandChan <- frame:
			case <-client.exitChan:
				return
			}
		}
	}
}

func (client *Client) waitResponse(seq uint16, timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			fmt.Println("timeout waiting for response")
			return
		case <-client.exitChan:
			return
		case res, ok := <-client.commandChan:
			if !ok {
				break
			}
			if res.Seq == seq {
				if res.Error != "" {
					fmt.Print(res.Error)
				} else {
					fmt.Print(string(res.Data))
				}
				if res.Flag == FlagComplete {
					fmt.Println("")
					return
				}
			}
		}
	}
}

func (client *Client) completer(str string) (ss []string) {
	var (
		err error
		seq uint16
	)
	ss = make([]string, 0)
	seq = client.getSequence()
	if err = writeFrame(client.conn, newFrame(PacketTypeCompleter, FlagComplete, seq, client.Timeout, []byte(str))); err != nil {
		return
	}
	select {
	case <-time.After(time.Second * 5):
	case frame, ok := <-client.completerChan:
		if ok {
			err = json.Unmarshal(frame.Data, &ss)
		}
	}
	return
}

func (client *Client) Execute(s string) (err error) {
	var (
		seq uint16
	)
	if client.conn, err = client.dialContext(client.ctx, client.address); err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
	}()
	go client.ioLoop(client.conn)
	seq = client.getSequence()
	if err = writeFrame(client.conn, newFrame(PacketTypeCommand, FlagComplete, seq, client.Timeout, []byte(s))); err != nil {
		return err
	}
	client.waitResponse(seq, client.Timeout)
	return
}

func (client *Client) Shell() (err error) {
	var (
		seq  uint16
		line string
	)
	client.liner.SetCtrlCAborts(true)
	if client.conn, err = client.dialContext(client.ctx, client.address); err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
	}()
	if err = writeFrame(client.conn, newFrame(PacketTypeHandshake, FlagComplete, client.getSequence(), client.Timeout, nil)); err != nil {
		return
	}
	go client.ioLoop(client.conn)
	select {
	case <-client.readyChan:
	case <-client.ctx.Done():
		return
	}
	client.liner.SetCompleter(client.completer)
	for {
		if line, err = client.liner.Prompt(client.name + "> "); err != nil {
			break
		}
		if atomic.LoadInt32(&client.exitFlag) == 1 {
			fmt.Println(Bye)
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch strings.ToLower(line) {
		case "exit", "quit":
			fmt.Println(Bye)
			return
		case "clear", "cls":
			fmt.Print("\033[2J")
			continue
		}
		seq = client.getSequence()
		if err = writeFrame(client.conn, newFrame(PacketTypeCommand, FlagComplete, seq, client.Timeout, []byte(line))); err != nil {
			break
		}
		client.liner.AppendHistory(line)
		client.waitResponse(seq, client.Timeout)
	}
	return
}

func (client *Client) Close() (err error) {
	if !atomic.CompareAndSwapInt32(&client.exitFlag, 0, 1) {
		return
	}
	close(client.exitChan)
	if client.conn != nil {
		err = client.conn.Close()
	}
	if client.liner != nil {
		err = client.liner.Close()
	}
	return
}

func NewClient(ctx context.Context, addr string) *Client {
	var (
		timeout time.Duration
	)
	if ctx == nil {
		ctx = context.Background()
	}
	timeout = time.Second * 30
	return &Client{
		ctx:           ctx,
		address:       addr,
		name:          filepath.Base(os.Args[0]),
		Timeout:       timeout,
		liner:         liner.NewLiner(),
		readyChan:     make(chan struct{}, 1),
		exitChan:      make(chan struct{}),
		commandChan:   make(chan *Frame, 5),
		completerChan: make(chan *Frame, 5),
	}
}
