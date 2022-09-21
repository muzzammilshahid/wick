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
	"github.com/tcnksm/go-input"
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

func getInputFromUser() (*core.ClientInfo, error) {
	ui := &input.UI{}
	clientInfo := &core.ClientInfo{}
	sectionName, err := ui.Ask("Enter section name", &input.Options{
		Default:   ini.DefaultSection,
		HideOrder: true,
	})
	clientInfo.Url, err = ui.Ask("Enter url", &input.Options{
		Default:   "ws://localhost:8080/ws",
		HideOrder: true,
	})
	clientInfo.Realm, err = ui.Ask("Enter realm", &input.Options{
		Default:   "realm1",
		HideOrder: true,
	})
	serializerStr, err := ui.Ask("Enter serializer", &input.Options{
		Default:   "json",
		Loop:      true,
		HideOrder: true,
		ValidateFunc: func(s string) error {
			if s != "json" && s != "cbor" && s != "msgpack" {
				return fmt.Errorf("value must be one of 'json', 'msgpack', 'cbor'")
			}
			return nil
		},
	})
	clientInfo.Authid, err = ui.Ask("Enter authid", &input.Options{
		HideOrder: true,
	})
	clientInfo.Authrole, err = ui.Ask("Enter authrole", &input.Options{
		HideOrder: true,
	})
	clientInfo.AuthMethod, err = ui.Ask("Enter authmethod", &input.Options{
		Default:   "anonymous",
		HideOrder: true,
		Loop:      true,
		ValidateFunc: func(s string) error {
			if s != "anonymous" && s != "ticket" && s != "wampcra" && s != "cryptosign" {
				return fmt.Errorf("value must be one of 'anonymous', 'ticket', 'wampcra', 'cryptosign'")
			}
			return nil
		},
	})
	if clientInfo.AuthMethod == "ticket" {
		clientInfo.Ticket, err = ui.Ask("Enter ticket", &input.Options{
			HideOrder: true,
		})
	} else if clientInfo.AuthMethod == "wampcra" {
		clientInfo.Secret, err = ui.Ask("Enter secret", &input.Options{
			HideOrder: true,
		})
	} else if clientInfo.AuthMethod == "cryptosign" {
		clientInfo.PrivateKey, err = ui.Ask("Enter private key", &input.Options{
			HideOrder: true,
		})
	}
	if err != nil {
		return nil, err
	}
	if err = writeProfile(sectionName, serializerStr, clientInfo); err != nil {
		return nil, err
	}
	return clientInfo, nil
}

func writeProfile(sectionName string, serializerStr string, clientInfo *core.ClientInfo) error {
	filePath := fmt.Sprintf("%s/.wick/config", os.Getenv("HOME"))
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		err = os.MkdirAll(fmt.Sprintf("%s/.wick", os.Getenv("HOME")), 0700)
		file, err := os.Create(filePath)
		if err != nil {
			return err
		}
		file.Close()
	}
	cfg, err := ini.Load(filePath)
	if err != nil {
		return fmt.Errorf("fail to load config: %v", err)
	}

	section, err := cfg.NewSection(sectionName)
	if err != nil {
		return fmt.Errorf("fail to create config: %v", err)
	}

	section, err = cfg.GetSection(sectionName)

	if _, err = section.NewKey("url", clientInfo.Url); err != nil {
		return fmt.Errorf("error in creating key: %v", err)
	}

	if _, err = section.NewKey("realm", clientInfo.Realm); err != nil {
		return fmt.Errorf("error in creating key: %v", err)
	}

	if _, err = section.NewKey("serializer", serializerStr); err != nil {
		return fmt.Errorf("error in creating key: %v", err)
	}

	if _, err = section.NewKey("authid", clientInfo.Authid); err != nil {
		return fmt.Errorf("error in creating key: %v", err)
	}

	if _, err = section.NewKey("authrole", clientInfo.Authrole); err != nil {
		return fmt.Errorf("error in creating key: %v", err)
	}

	if _, err = section.NewKey("authmethod", clientInfo.AuthMethod); err != nil {
		return fmt.Errorf("error in creating key: %v", err)
	}

	if _, err = section.NewKey("private-key", clientInfo.PrivateKey); err != nil {
		return fmt.Errorf("error in creating key: %v", err)
	}

	if _, err = section.NewKey("ticket", clientInfo.Ticket); err != nil {
		return fmt.Errorf("error in creating key: %v", err)
	}

	if _, err = section.NewKey("secret", clientInfo.Secret); err != nil {
		return fmt.Errorf("error in creating key: %v", err)
	}

	err = cfg.SaveTo(filePath)
	if err != nil {
		return fmt.Errorf("error in saving file: %s", err)
	}
	return nil
}

func readFromProfile(profile string) (*core.ClientInfo, error) {
	clientInfo := &core.ClientInfo{}
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
	if section.Key("serializer").String() != "json" &&
		section.Key("serializer").String() != "msgpack" &&
		section.Key("serializer").String() != "cbor" &&
		section.Key("serializer").String() != "" {
		return nil, fmt.Errorf("serailizer must be json, msgpack or cbor")
	}
	clientInfo.Serializer = getSerializerByName(section.Key("serializer").Validate(func(s string) string {
		if len(s) == 0 {
			return "json"
		}
		return s
	}))
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
	var session *client.Client
	var err error
	wp := workerpool.New(concurrency)
	resC := make(chan error, sessionCount)
	for i := 0; i < sessionCount; i++ {
		wp.Submit(func() {
			session, err = connect(clientInfo, logTime, keepalive)
			mutex.Lock()
			sessions = append(sessions, session)
			mutex.Unlock()
			resC <- err
		})
	}

	wp.StopWait()
	if err = getErrorFromErrorChannel(resC); err != nil {
		return nil, err
	}
	return sessions, nil
}
