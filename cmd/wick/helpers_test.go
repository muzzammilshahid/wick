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
	"io"
	"testing"
	"time"

	"github.com/gammazero/nexus/v3/router"
	"github.com/gammazero/nexus/v3/transport/serialize"
	"github.com/gammazero/nexus/v3/wamp"
	"github.com/gammazero/workerpool"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/s-things/wick/core"
)

var (
	testRealm      = "wick.test"
	netAddr        = "localhost"
	wsPort         = 8080
	testClientInfo = &core.ClientInfo{
		Url:        "ws://localhost:8080/ws",
		Realm:      testRealm,
		Serializer: serialize.JSON,
		AuthMethod: "anonymous",
	}
	sessionCount    = 1000
	testConcurrency = 100
)

var logger *log.Logger

func init() {
	logger = log.New()
}

func startWsServer() (router.Router, io.Closer, error) {
	realmConfig := &router.RealmConfig{
		URI:              wamp.URI(testRealm),
		StrictURI:        true,
		AnonymousAuth:    true,
		AllowDisclose:    true,
		RequireLocalAuth: true,
	}
	config := &router.Config{
		RealmConfigs: []*router.RealmConfig{realmConfig},
	}
	rout, err := router.NewRouter(config, logger)
	if err != nil {
		return nil, nil, err
	}
	// Create websocket server.
	wss := router.NewWebsocketServer(rout)
	wsAddr := fmt.Sprintf("%s:%d", netAddr, wsPort)
	wsCloser, err := wss.ListenAndServe(wsAddr)
	if err != nil {
		return nil, nil, err
	}
	return rout, wsCloser, err
}

func TestSessions(t *testing.T) {
	rout, wsCloser, err := startWsServer()
	require.NoError(t, err)
	defer rout.Close()
	defer wsCloser.Close()

	t.Run("TestConnect", func(t *testing.T) {
		session, err := connect(testClientInfo, false, 1)
		defer session.Close()
		require.NoError(t, err)
		require.Equal(t, true, session.Connected(), "get already closed session")
	})

	t.Run("TestGetSessions", func(t *testing.T) {
		sessions, err := getSessions(testClientInfo, sessionCount, testConcurrency, false, 0)
		defer func() {
			wp := workerpool.New(len(sessions))
			for _, sess := range sessions {
				s := sess
				wp.Submit(func() {
					// Close the connection to the router
					s.Close()
				})
			}
			wp.StopWait()
		}()
		require.NoError(t, err)
		require.Equal(t, sessionCount, len(sessions))
	})

	t.Run("TestConcurrency", func(t *testing.T) {
		startTime := time.Now().UnixMilli()
		sessions, err := getSessions(testClientInfo, sessionCount, 1, false, 0)
		timeSeq := time.Now().UnixMilli() - startTime
		defer func() {
			wp := workerpool.New(len(sessions))
			for _, sess := range sessions {
				s := sess
				wp.Submit(func() {
					// Close the connection to the router
					s.Close()
				})
			}
			wp.StopWait()
		}()
		require.NoError(t, err)

		startTime = time.Now().UnixMilli()
		sessions, err = getSessions(testClientInfo, sessionCount, 1000, false, 0)
		timeCon := time.Now().UnixMilli() - startTime
		defer func() {
			wp := workerpool.New(len(sessions))
			for _, sess := range sessions {
				s := sess
				wp.Submit(func() {
					// Close the connection to the router
					s.Close()
				})
			}
			wp.StopWait()
		}()
		require.NoError(t, err)
		require.Greater(t, timeSeq, timeCon, "concurrent calls must take less time")
	})
}

func TestSerializerSelect(t *testing.T) {
	for _, data := range []struct {
		name               string
		expectedSerializer serialize.Serialization
		message            string
	}{
		{"json", serialize.JSON, fmt.Sprintf("invalid serializer id for json, expected=%d", serialize.JSON)},
		{"cbor", serialize.CBOR, fmt.Sprintf("invalid serializer id for cbor, expected=%d", serialize.CBOR)},
		{"msgpack", serialize.MSGPACK, fmt.Sprintf("invalid serializer id for msgpack, expected=%d", serialize.MSGPACK)},
		{"halo", -1, "should not accept as only anonymous,ticket,wampcra,cryptosign are allowed"},
	} {
		serializerId := getSerializerByName(data.name)
		assert.Equal(t, data.expectedSerializer, serializerId, data.message)
	}
}

func TestSelectAuthMethod(t *testing.T) {
	for _, data := range []struct {
		privateKey     string
		ticket         string
		secret         string
		expectedMethod string
	}{
		{"b99067e6e271ae300f3f5d9809fa09288e96f2bcef8dd54b7aabeb4e579d37ef", "", "", "cryptosign"},
		{"", "williamsburg", "", "ticket"},
		{"", "", "williamsburg", "wampcra"},
		{"", "", "", "anonymous"},
	} {
		method := selectAuthMethod(data.privateKey, data.ticket, data.secret)
		assert.Equal(t, data.expectedMethod, method, "problem in choosing auth method")
	}
}
