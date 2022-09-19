/*
*
* Copyright 2021-2022 Simple Things Inc.
*
* Permission is hereby granted, free of charge, to any person obtaining a copy
* of this software and associated documentation files (the "Software"), to deal
* in the Software without restriction, including without limitation the rights
* to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
* copies of the Software, and to permit persons to whom the Software is
* furnished to do so, subject to the following conditions:
*
* The above copyright notice and this permission notice shall be included in all
* copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
* IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
* FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
* AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
* LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
* OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
* SOFTWARE.
*
 */

package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gammazero/nexus/v3/client"
	"github.com/gammazero/nexus/v3/transport/serialize"
	"github.com/gammazero/workerpool"
	log "github.com/sirupsen/logrus"
	"gopkg.in/ini.v1"

	"github.com/s-things/wick/core"
)

var (
	coreConnectAnonymous  = core.ConnectAnonymous
	coreConnectTicket     = core.ConnectTicket
	coreConnectCRA        = core.ConnectCRA
	coreConnectCryptoSign = core.ConnectCryptoSign
)

func getSerializerByName(name string) serialize.Serialization {
	switch name {
	case "json":
		return serialize.JSON
	case "msgpack":
		return serialize.MSGPACK
	case "cbor":
		return serialize.CBOR
	}
	return -1
}

func selectAuthMethod(privateKey string, ticket string, secret string) string {
	if privateKey != "" && (ticket == "" && secret == "") {
		return "cryptosign"
	} else if ticket != "" && (privateKey == "" && secret == "") {
		return "ticket"
	} else if secret != "" && (privateKey == "" && ticket == "") {
		return "wampcra"
	}

	return "anonymous"
}

func validateData(sessionCount int, concurrency int, keepAlive int) error {
	if sessionCount < 1 {
		return fmt.Errorf("parallel must be greater than zero")
	}
	if concurrency < 1 {
		return fmt.Errorf("concurrency must be greater than zero")
	}
	if keepAlive < 0 {
		return fmt.Errorf("keepalive interval must be greater than zero")
	}

	return nil
}

func readFromProfile(profile string) (*core.ClientInfo, error) {
	var clientInfo *core.ClientInfo
	cfg, err := ini.Load(fmt.Sprintf("%s/.wick/config", os.Getenv("HOME")))
	if err != nil {
		return nil, fmt.Errorf("fail to read config: %v", err)
	}

	section, err := cfg.GetSection(profile)
	if err != nil {
		return nil, fmt.Errorf("error in getting section: %s", err)
	}

	clientInfo.Url = section.Key("url").Validate(func(s string) string {
		if len(s) == 0 {
			return "ws://localhost:8080/ws"
		}
		return s
	})
	clientInfo.Realm = section.Key("realm").Validate(func(s string) string {
		if len(s) == 0 {
			return "realm1"
		}
		return s
	})
	clientInfo.Authid = section.Key("authid").String()
	clientInfo.Authrole = section.Key("authrole").String()
	clientInfo.AuthMethod = section.Key("authmethod").String()
	if clientInfo.AuthMethod == "cryptosign" {
		clientInfo.PrivateKey = section.Key("private-key").String()
	} else if clientInfo.AuthMethod == "ticket" {
		clientInfo.Ticket = section.Key("ticket").String()
	} else if clientInfo.AuthMethod == "wampcra" {
		clientInfo.Secret = section.Key("secret").String()
	}

	return clientInfo, nil
}

func getErrorFromErrorChannel(resC chan error) error {
	close(resC)
	var errs []string
	for err := range resC {
		if err != nil {
			errs = append(errs, fmt.Sprintf("- %v", err))
		}
	}
	if len(errs) != 0 {
		return fmt.Errorf("got errors:\n%v", strings.Join(errs, "\n"))
	}
	return nil
}

func connect(clientInfo *core.ClientInfo, logTime bool, keepalive int) (*client.Client, error) {
	var session *client.Client
	var err error
	var startTime int64

	if logTime {
		startTime = time.Now().UnixMilli()
	}
	switch clientInfo.AuthMethod {
	case "anonymous":
		if clientInfo.PrivateKey != "" {
			return nil, fmt.Errorf("private key not needed for anonymous auth")
		}
		if clientInfo.Ticket != "" {
			return nil, fmt.Errorf("ticket not needed for anonymous auth")
		}
		if clientInfo.Secret != "" {
			return nil, fmt.Errorf("secret not needed for anonymous auth")
		}
		session, err = coreConnectAnonymous(clientInfo, keepalive)
		if err != nil {
			return nil, err
		}
	case "ticket":
		if clientInfo.Ticket == "" {
			return nil, fmt.Errorf("must provide ticket when authMethod is ticket")
		}
		session, err = coreConnectTicket(clientInfo, keepalive)
		if err != nil {
			return nil, err
		}
	case "wampcra":
		if clientInfo.Secret == "" {
			return nil, fmt.Errorf("must provide secret when authMethod is wampcra")
		}
		session, err = coreConnectCRA(clientInfo, keepalive)
		if err != nil {
			return nil, err
		}
	case "cryptosign":
		if clientInfo.PrivateKey == "" {
			return nil, fmt.Errorf("must provide private key when authMethod is cryptosign")
		}
		session, err = coreConnectCryptoSign(clientInfo, keepalive)
		if err != nil {
			return nil, err
		}
	}

	if logTime {
		endTime := time.Now().UnixMilli()
		log.Printf("session joined in %dms\n", endTime-startTime)
	}
	return session, err
}

func getSessions(clientInfo *core.ClientInfo, sessionCount int, concurrency int,
	logTime bool, keepalive int) ([]*client.Client, error) {
	var sessions []*client.Client
	var mutex sync.Mutex
	wp := workerpool.New(concurrency)
	resC := make(chan error, sessionCount)
	for i := 0; i < sessionCount; i++ {
		wp.Submit(func() {
			session, err := connect(clientInfo, logTime, keepalive)
			mutex.Lock()
			sessions = append(sessions, session)
			mutex.Unlock()
			resC <- err
		})
	}

	wp.StopWait()
	if err := getErrorFromErrorChannel(resC); err != nil {
		return nil, err
	}
	return sessions, nil
}
