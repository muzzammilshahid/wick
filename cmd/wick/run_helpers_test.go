package main_test

import (
	"testing"
	"time"

	"github.com/gammazero/nexus/v3/client"
	"github.com/gammazero/nexus/v3/router"
	"github.com/gammazero/nexus/v3/wamp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	main "github.com/s-things/wick/cmd/wick"
)

const (
	testProcedure  = "com.procedure.test"
	testProcedure1 = "com.procedure.test1"

	testTopic  = "com.topic.test"
	testTopic1 = "com.topic.test1"
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

func TestEqualArgsKwargs(t *testing.T) {
	for _, data := range []struct {
		list1          wamp.List
		list2          wamp.List
		dict1          wamp.Dict
		dict2          wamp.Dict
		expectedOutput bool
	}{
		{wamp.List{"foo", 1, "OK"}, wamp.List{"foo", 1, "OK", "check"},
			wamp.Dict{"key1": "value1"}, wamp.Dict{"key1": "value1"},
			false},
		{wamp.List{"foo", 1, "OK"}, wamp.List{"foo", 1, "OK"},
			wamp.Dict{"key1": "value1", "key2": "2"}, wamp.Dict{"key1": "value1"},
			false},
		{wamp.List{"foo", 1, "OK"}, wamp.List{"foo", 1, "OK"},
			wamp.Dict{"key1": "value1", "key2": "2"}, wamp.Dict{"key1": "value1", "key2": "2"},
			true},
		{wamp.List{"foo", 1, "OK"}, wamp.List{"foo", 1, "OK", "check"},
			wamp.Dict{"key1": "value1", "key2": "2"}, wamp.Dict{"key1": "value1"},
			false},
	} {
		isEqual := main.EqualArgsKwargs(data.list1, data.list2, data.dict1, data.dict2)
		assert.Equal(t, data.expectedOutput, isEqual)
	}
}

func TestRunTasks(t *testing.T) {
	producerSession, consumerSession, rout, err := connectedTestClients()
	require.NoError(t, err)
	defer producerSession.Close()
	defer consumerSession.Close()
	defer rout.Close()

	compose := main.Compose{
		Version: "2.0",
		Tasks: []main.Tasks{
			{Name: "register a cool procedure", Type: main.Register, Procedure: testProcedure},
			{Name: "register second procedure", Type: main.Register, Procedure: testProcedure1,
				Options: wamp.Dict{"invoke": "roundrobin"}, Yield: &main.ArgsKwargs{
					Args: wamp.List{"Hello", "ok"}, Kwargs: wamp.Dict{"key": "value"},
				}, Invocation: &main.ArgsKwargs{Args: wamp.List{"Hello", "ok"}, Kwargs: wamp.Dict{"key": "value"}}},

			{Name: "call a procedure", Type: main.Call, Procedure: testProcedure1,
				Result: &main.ArgsKwargs{Args: wamp.List{"Hello", "ok"}, Kwargs: wamp.Dict{"key": "value"}}},
			{Name: "call a procedure", Type: main.Call, Procedure: testProcedure1,
				Result:     &main.ArgsKwargs{Args: wamp.List{"Hello", "ok"}, Kwargs: wamp.Dict{"key": "value"}},
				Parameters: &main.ArgsKwargs{Args: wamp.List{"Hello", "ok"}, Kwargs: wamp.Dict{"key": "value"}}},

			{Name: "Subscribe to a topic", Type: main.Subscribe, Topic: testTopic},
			{Name: "Subscribe to second topic", Type: main.Subscribe, Topic: testTopic1,
				Options: wamp.Dict{"match": "wildcard"}, Event: &main.ArgsKwargs{
					Args: wamp.List{"Hello", "ok"}, Kwargs: wamp.Dict{"key": "value"},
				}},

			{Name: "publish to topic", Type: main.Publish, Topic: testTopic1},
			{Name: "publish to topic", Type: main.Publish, Topic: testTopic1,
				Parameters: &main.ArgsKwargs{Args: wamp.List{"Hello", "ok"}, Kwargs: wamp.Dict{"key": "value"}}},
		},
	}
	err = main.ExecuteTasks(compose, producerSession, consumerSession)
	assert.NoError(t, err)

	for _, invalidCompose := range []main.Compose{
		{
			Version: "2.0",
			Tasks: []main.Tasks{
				{Name: "register a cool procedure", Type: main.Register, Procedure: testProcedure, Topic: testTopic1},
			},
		},
		{
			Version: "2.0",
			Tasks: []main.Tasks{
				{Name: "register second procedure", Type: main.Register, Procedure: testProcedure1,
					Options: wamp.Dict{"invoke": "roundrobin"}, Yield: &main.ArgsKwargs{
						Args: wamp.List{"Hello", "ok"}, Kwargs: wamp.Dict{"key": "value"},
					}, Event: &main.ArgsKwargs{Args: wamp.List{"Hello", "ok"}, Kwargs: wamp.Dict{"key": "value"}}},
			}},
		{
			Version: "2.0",
			Tasks: []main.Tasks{
				{Name: "call a procedure", Type: main.Call, Procedure: testProcedure1,
					Yield: &main.ArgsKwargs{Args: wamp.List{"Hello", "ok"}, Kwargs: wamp.Dict{"key": "value"}}},
			}},
		{
			Version: "2.0",
			Tasks: []main.Tasks{
				{Name: "call a procedure", Type: main.Call, Procedure: testProcedure1,
					Result:     &main.ArgsKwargs{Args: wamp.List{"Hello", "ok"}, Kwargs: wamp.Dict{"key": "value"}},
					Invocation: &main.ArgsKwargs{Args: wamp.List{"Hello"}, Kwargs: wamp.Dict{"key": "value"}}},
			}},
		{
			Version: "2.0",
			Tasks: []main.Tasks{
				{Name: "Subscribe to a topic", Type: "hello", Procedure: testTopic},
			}},
		{
			Version: "2.0",
			Tasks: []main.Tasks{{Name: "Subscribe to second topic", Topic: testTopic1,
				Options: wamp.Dict{"match": "wildcard"}, Event: &main.ArgsKwargs{
					Args: wamp.List{"Hello", "ok"}, Kwargs: wamp.Dict{"key": "value"},
				}},
			}},
		{
			Version: "2.0",
			Tasks:   []main.Tasks{{Name: "publish to topic", Type: main.Call, Topic: testProcedure1}}},
		{
			Version: "2.0",
			Tasks: []main.Tasks{{Name: "publish to topic", Procedure: testProcedure1,
				Parameters: &main.ArgsKwargs{Args: wamp.List{"Hello", "ok"}, Kwargs: wamp.Dict{"key": "value"}}}}},
	} {
		err = main.ExecuteTasks(invalidCompose, producerSession, consumerSession)
		assert.Error(t, err)
	}
}

func TestValidateRegister(t *testing.T) {
	err := main.ValidateRegister(testProcedure, "", nil, nil, nil)
	assert.NoError(t, err)

	for _, invalidData := range []struct {
		procedure  string
		topic      string
		event      *main.ArgsKwargs
		result     *main.ArgsKwargs
		parameters *main.ArgsKwargs
		errorText  string
	}{
		{testProcedure, testTopic1, nil, nil, nil,
			"topic is not required for register"},
		{testProcedure, "", &main.ArgsKwargs{}, nil, nil,
			"event is not required for register"},
		{testProcedure, "", nil, &main.ArgsKwargs{}, nil,
			"result is not required for register"},
		{testProcedure, "", nil, nil, &main.ArgsKwargs{},
			"parameters are not required for register"},
	} {
		err = main.ValidateRegister(invalidData.procedure, invalidData.topic, invalidData.event,
			invalidData.result, invalidData.parameters)
		assert.Error(t, err)
		assert.EqualError(t, err, invalidData.errorText)
	}
}

func TestValidateCall(t *testing.T) {
	err := main.ValidateCall(testProcedure, "", nil, nil, nil)
	assert.NoError(t, err)

	for _, invalidData := range []struct {
		procedure  string
		topic      string
		event      *main.ArgsKwargs
		yield      *main.ArgsKwargs
		invocation *main.ArgsKwargs
		errorText  string
	}{
		{testProcedure, "foo", nil, nil, nil,
			"topic is not required for call"},
		{testProcedure, "", &main.ArgsKwargs{}, nil, nil,
			"event is not required for call"},
		{testProcedure, "", nil, &main.ArgsKwargs{}, nil,
			"yield is not required for call"},
		{testProcedure, "", nil, nil, &main.ArgsKwargs{},
			"invocation are not required for call"},
	} {
		err = main.ValidateCall(invalidData.procedure, invalidData.topic, invalidData.event,
			invalidData.yield, invalidData.invocation)
		assert.Error(t, err)
		assert.EqualError(t, err, invalidData.errorText)
	}
}

func TestValidateSubscribe(t *testing.T) {
	err := main.ValidateSubscribe(testTopic, "", nil, nil, nil, nil)
	assert.NoError(t, err)

	for _, invalidData := range []struct {
		topic      string
		procedure  string
		result     *main.ArgsKwargs
		yield      *main.ArgsKwargs
		invocation *main.ArgsKwargs
		parameters *main.ArgsKwargs
		errorText  string
	}{
		{testTopic, "foo", nil, nil, nil, nil,
			"procedure is not required for subscribe"},
		{testTopic, "", &main.ArgsKwargs{}, nil, nil, nil,
			"result is not required for subscribe"},
		{testTopic, "", nil, &main.ArgsKwargs{}, nil, nil,
			"yield is not required for subscribe"},
		{testTopic, "", nil, nil, &main.ArgsKwargs{}, nil,
			"invocation is not required for subscribe"},
		{testTopic, "", nil, nil, nil, &main.ArgsKwargs{},
			"parameters are not required for subscribe"},
	} {
		err = main.ValidateSubscribe(invalidData.topic, invalidData.procedure, invalidData.result,
			invalidData.yield, invalidData.invocation, invalidData.parameters)
		assert.Error(t, err)
		assert.EqualError(t, err, invalidData.errorText)
	}
}

func TestValidatePublish(t *testing.T) {
	err := main.ValidatePublish(testTopic, "", nil, nil, nil, nil)
	assert.NoError(t, err)

	for _, invalidData := range []struct {
		topic      string
		procedure  string
		event      *main.ArgsKwargs
		yield      *main.ArgsKwargs
		invocation *main.ArgsKwargs
		result     *main.ArgsKwargs
		errorText  string
	}{
		{testTopic, "foo", nil, nil, nil, nil,
			"procedure is not required for publish"},
		{testTopic, "", &main.ArgsKwargs{}, nil, nil, nil,
			"event is not required for publish"},
		{testTopic, "", nil, &main.ArgsKwargs{}, nil, nil,
			"yield is not required for publish"},
		{testTopic, "", nil, nil, &main.ArgsKwargs{}, nil,
			"invocation is not required for publish"},
		{testTopic, "", nil, nil, nil, &main.ArgsKwargs{},
			"result is not required for publish"},
	} {
		err = main.ValidatePublish(invalidData.topic, invalidData.procedure, invalidData.event,
			invalidData.yield, invalidData.invocation, invalidData.result)
		assert.Error(t, err)
		assert.EqualError(t, err, invalidData.errorText)
	}
}
