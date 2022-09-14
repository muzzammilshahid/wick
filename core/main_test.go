package core

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	"testing"
	"time"

	"github.com/gammazero/nexus/v3/client"
	"github.com/gammazero/nexus/v3/router"
	"github.com/gammazero/nexus/v3/wamp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testRealm     = "wick.test"
	testProcedure = "wick.test.procedure"
	testTopic     = "wick.test.topic"
	repeatCount   = 1000
	repeatPublish = 100000
	delay         = 1000
)

func getTestRouter() (router.Router, error) {
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
	return router.NewRouter(config, log.New())
}

func newTestClient(r router.Router) (*client.Client, error) {
	clientConfig := &client.Config{
		Realm:           testRealm,
		ResponseTimeout: 500 * time.Millisecond,
		Logger:          log.New(),
		Debug:           false,
	}
	return client.ConnectLocal(r, *clientConfig)
}

func connectedTestClients() (*client.Client, *client.Client, router.Router, error) {
	r, err := getTestRouter()
	if err != nil {
		return nil, nil, nil, err
	}

	c1, err := newTestClient(r)
	if err != nil {
		return nil, nil, nil, err
	}
	c2, err := newTestClient(r)
	if err != nil {
		return nil, nil, nil, err
	}
	return c1, c2, r, nil
}

func TestRegisterDelay(t *testing.T) {
	rout, err := getTestRouter()
	assert.NoError(t, err, fmt.Sprintf("error in getting router: %s\n", err))
	defer rout.Close()

	session, err := newTestClient(rout)
	assert.NoError(t, err, fmt.Sprintf("error in getting session: %s\n", err))
	defer session.Close()

	go func() {
		err = Register(session, testProcedure, "", delay, 0, nil, false)
		assert.NoError(t, err, fmt.Sprintf("error in registering procedure: %s\n", err))
	}()

	err = session.Unregister(testProcedure)
	assert.Error(t, err, "procedure should register after 1 second")

	time.Sleep(1100 * time.Millisecond)
	err = session.Unregister(testProcedure)
	assert.NoError(t, err, "procedure not even register after delay")
}

func TestRegisterInvokeCount(t *testing.T) {
	invokeCount := 2
	sessionRegister, sessionCall, rout, err := connectedTestClients()
	require.NoError(t, err)
	defer sessionRegister.Close()
	defer sessionCall.Close()
	defer rout.Close()

	err = Register(sessionRegister, testProcedure, "", 0, invokeCount, nil, false)
	require.NoError(t, err, fmt.Sprintf("error in registering procedure: %s\n", err))

	for i := 0; i < invokeCount; i++ {
		_, err = sessionCall.Call(context.Background(), testProcedure, nil, nil, nil, nil)
		require.NoError(t, err, fmt.Sprintf("error in calling procedure: %s\n", err))
	}
	err = sessionRegister.Unregister(testProcedure)
	require.Error(t, err, "procedure not unregister after invoke-count")
}

func TestRegisterOnInvocationCmd(t *testing.T) {
	sessionRegister, sessionCall, rout, err := connectedTestClients()
	require.NoError(t, err)
	defer sessionRegister.Close()
	defer sessionCall.Close()
	defer rout.Close()

	err = Register(sessionRegister, testProcedure, "pwd", 0, 0, nil, false)
	require.NoError(t, err, fmt.Sprintf("error in registering procedure: %s\n", err))

	result, err := sessionCall.Call(context.Background(), testProcedure, nil, nil, nil, nil)
	require.NoError(t, err, fmt.Sprintf("error in calling procedure: %s\n", err))

	out, _, _ := shellOut("pwd")
	require.Equal(t, out, result.Arguments[0], "invalid call results")
}

func TestCallDelayRepeatConcurrency(t *testing.T) {
	sessionRegister, sessionCall, rout, err := connectedTestClients()
	require.NoError(t, err)
	defer sessionRegister.Close()
	defer sessionCall.Close()
	defer rout.Close()

	iterator := 0
	invocationHandler := func(ctx context.Context, inv *wamp.Invocation) client.InvokeResult {
		iterator++
		return client.InvokeResult{Args: wamp.List{}}
	}

	err = sessionRegister.Register(testProcedure, invocationHandler, nil)
	require.NoError(t, err, fmt.Sprintf("error in registering procedure: %s\n", err))
	defer sessionRegister.Unregister(testProcedure)

	t.Run("TestCallDelay", func(t *testing.T) {
		go func() {
			err = Call(sessionCall, testProcedure, []string{"Hello", "1"}, nil, false, 1, 1000, 0, nil)
			require.NoError(t, err, fmt.Sprintf("error in calling procedure: %s\n", err))
		}()
		require.Equal(t, 0, iterator, "procedure called without delay")
		time.Sleep(1100 * time.Millisecond)
		require.Equal(t, 1, iterator, "procedure not even called after delay")
		iterator = 0
	})

	var timeRepeat int64
	t.Run("TestCallRepeat", func(t *testing.T) {
		startTime := time.Now().UnixMilli()
		err = Call(sessionCall, testProcedure, []string{"Hello", "1"}, nil, false, repeatCount, 0, 0, nil)
		timeRepeat = time.Now().UnixMilli() - startTime
		require.NoError(t, err, fmt.Sprintf("error in calling procedure: %s\n", err))
		require.Equal(t, 1000, iterator, "procedure not correctly called repeatedly")
	})

	t.Run("TestCallConcurrency", func(t *testing.T) {
		startTime := time.Now().UnixMilli()
		err = Call(sessionCall, testProcedure, []string{"Hello", "1"}, nil, false, repeatCount, 0, 100, nil)
		timeConcurrentCalls := time.Now().UnixMilli() - startTime
		require.NoError(t, err, fmt.Sprintf("error in calling procedure: %s\n", err))
		require.Greater(t, timeRepeat, timeConcurrentCalls, "concurrency not works correctly")
	})
}

func TestSubscribe(t *testing.T) {
	rout, err := getTestRouter()
	assert.NoError(t, err, fmt.Sprintf("error in getting router: %s\n", err))
	defer rout.Close()

	session, err := newTestClient(rout)
	assert.NoError(t, err, fmt.Sprintf("error in getting session: %s\n", err))
	defer session.Close()

	err = Subscribe(session, testTopic, nil, false, false)
	require.NoError(t, err, fmt.Sprintf("error in subscribing: %s\n", err))

	err = session.Unsubscribe(testTopic)
	require.NoError(t, err, fmt.Sprintf("error in subscribing: %s\n", err))
}

func TestPublishDelayRepeatConcurrency(t *testing.T) {
	sessionSubscribe, sessionPublish, rout, err := connectedTestClients()
	require.NoError(t, err)
	defer sessionSubscribe.Close()
	defer sessionPublish.Close()
	defer rout.Close()

	iterator := 0
	eventHandler := func(event *wamp.Event) {
		iterator++
	}

	err = sessionSubscribe.Subscribe(testTopic, eventHandler, nil)
	require.NoError(t, err, fmt.Sprintf("error in subscribing topic: %s\n", err))
	defer sessionSubscribe.Unsubscribe(testTopic)

	t.Run("TestPublishDelay", func(t *testing.T) {
		go func() {
			err = Publish(sessionPublish, testTopic, nil, nil, nil, false, 1, 1000, 1)
			require.NoError(t, err, fmt.Sprintf("error in publishing: %s\n", err))
		}()
		require.Equal(t, 0, iterator, "topic published without delay")
		time.Sleep(1100 * time.Millisecond)
		require.Equal(t, 1, iterator, "topic not even published after delay")
		iterator = 0
	})

	var timeRepeat int64
	t.Run("TestPublishRepeat", func(t *testing.T) {
		startTime := time.Now().UnixMilli()
		err = Publish(sessionPublish, testTopic, []string{"Hello", "1"}, nil, nil, false, repeatPublish, 0, 1)
		timeRepeat = time.Now().UnixMilli() - startTime
		require.NoError(t, err, fmt.Sprintf("error in publishing topic: %s\n", err))
		require.Equal(t, repeatPublish, iterator, "topic not correctly publish repeatedly")
	})

	t.Run("TestPublishConcurrency", func(t *testing.T) {
		startTime := time.Now().UnixMilli()
		err = Publish(sessionPublish, testTopic, []string{"Hello", "1"}, nil, nil, false, repeatPublish, 0, 10000)
		timeConcurrentCalls := time.Now().UnixMilli() - startTime
		require.NoError(t, err, fmt.Sprintf("error in publish ro topic: %s\n", err))
		require.Greater(t, timeRepeat, timeConcurrentCalls, "concurrency not works correctly")
	})
}
