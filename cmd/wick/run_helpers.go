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
	"context"
	"fmt"
	"reflect"

	"github.com/gammazero/nexus/v3/client"
	"github.com/gammazero/nexus/v3/wamp"
	log "github.com/sirupsen/logrus"
)

const (
	register  = "register"
	call      = "call"
	subscribe = "subscribe"
	publish   = "publish"
)

type argsKwargs struct {
	Args   wamp.List `yaml:"args"`
	Kwargs wamp.Dict `yaml:"kwargs"`
}

type Compose struct {
	Version string  `yaml:"version"`
	Tasks   []Tasks `yaml:"tasks"`
}

type Tasks struct {
	Name       string      `yaml:"name"`
	Type       string      `yaml:"type"`
	Options    wamp.Dict   `yaml:"options"`
	Procedure  string      `yaml:"procedure"`
	Yield      *argsKwargs `yaml:"yield"`
	Invocation *argsKwargs `yaml:"invocation"`
	Parameters *argsKwargs `yaml:"parameters"`
	Result     *argsKwargs `yaml:"result"`
	Topic      string      `yaml:"topic"`
	Event      *argsKwargs `yaml:"event"`
}

func equalArgsKwargs(list1, list2 wamp.List, dict1, dict2 wamp.Dict) bool {
	if isEqual := reflect.DeepEqual(list1, list2); !isEqual {
		return false
	}

	return reflect.DeepEqual(dict1, dict2)
}

func invocationHandler(invoke, yield *argsKwargs) func(ctx context.Context,
	invocation *wamp.Invocation) client.InvokeResult {
	return func(ctx context.Context, invocation *wamp.Invocation) client.InvokeResult {
		if invoke != nil {
			if isEqual := equalArgsKwargs(invoke.Args, invocation.Arguments, invoke.Kwargs,
				invocation.ArgumentsKw); !isEqual {
				log.Errorf("actual invocation is not equal to expected invocation: expected=%v %v actual=%s %s",
					invoke.Args, invoke.Kwargs, invocation.Arguments, invocation.ArgumentsKw)
			}
		}
		log.Debugf("procedure called with args:%s and kwarg:%s", invocation.Arguments, invocation.ArgumentsKw)
		if yield != nil {
			return client.InvokeResult{Args: yield.Args, Kwargs: yield.Kwargs}
		}
		return client.InvokeResult{}
	}
}

// executeTasks execute all the tasks in compose.
func executeTasks(compose Compose, producerSession, consumerSession *client.Client) error {
	for _, task := range compose.Tasks {
		switch task.Type {
		case register:
			if err := validateRegister(task.Procedure, task.Topic, task.Event, task.Result, task.Parameters); err != nil {
				return err
			}
			yield := task.Yield
			invoke := task.Invocation
			if err := producerSession.Register(task.Procedure, invocationHandler(invoke, yield), task.Options); err != nil {
				return err
			}
			log.Printf("Register to procedure %s", task.Procedure)

		case call:
			if err := validateCall(task.Procedure, task.Topic, task.Event, task.Yield, task.Invocation); err != nil {
				return err
			}
			var result *wamp.Result
			var err error
			if task.Parameters == nil {
				result, err = consumerSession.Call(context.Background(), task.Procedure, task.Options, nil,
					nil, nil)
			} else {
				result, err = consumerSession.Call(context.Background(), task.Procedure, task.Options, task.Parameters.Args,
					task.Parameters.Kwargs, nil)
			}
			if err != nil {
				return err
			}
			if task.Result != nil {
				if isEqual := equalArgsKwargs(task.Result.Args, result.Arguments, task.Result.Kwargs,
					result.ArgumentsKw); !isEqual {
					log.Errorf("actual call result is not equal to expected call result: expected=%v %v actual=%s %s",
						task.Result.Args, task.Result.Kwargs, result.Arguments, result.ArgumentsKw)
				}
			}
			log.Printf("Called procedure %s", task.Procedure)
			log.Debugf("call results: args:%s kwargs%s", result.Arguments, result.ArgumentsKw)

		case subscribe:
			if err := validateSubscribe(task.Topic, task.Procedure, task.Result, task.Yield, task.Invocation,
				task.Parameters); err != nil {
				return err
			}
			e := task.Event
			if err := producerSession.Subscribe(task.Topic, func(event *wamp.Event) {
				if e != nil {
					if isEqual := equalArgsKwargs(e.Args, event.Arguments, e.Kwargs, event.ArgumentsKw); !isEqual {
						log.Errorf("actual event is not equal to expected event: expected=%v %v actual=%s %s",
							e.Args, e.Kwargs, event.Arguments, event.ArgumentsKw)
					}
				}
			}, task.Options); err != nil {
				return err
			}
			log.Printf("Subscribe to topic %s", task.Topic)

		case publish:
			if err := validatePublish(task.Topic, task.Procedure, task.Event, task.Yield, task.Invocation,
				task.Result); err != nil {
				return err
			}

			var err error
			if task.Parameters == nil {
				err = consumerSession.Publish(task.Topic, task.Options, nil, nil)
			} else {
				err = consumerSession.Publish(task.Topic, task.Options, task.Parameters.Args, task.Parameters.Kwargs)
			}
			if err != nil {
				return err
			}
			log.Printf("Publish to topic %s", task.Topic)

		default:
			return fmt.Errorf("%s not supported: supported types are %s, %s, %s, %s",
				task.Type, register, call, subscribe, publish)
		}
	}
	return nil
}

func validateRegister(procedure, topic string, event, result, parameters *argsKwargs) error {
	if procedure == "" {
		return fmt.Errorf("procedure is required for register")
	}
	if topic != "" {
		return fmt.Errorf("topic is not required for register")
	}
	if event != nil {
		return fmt.Errorf("event is not required for register")
	}
	if result != nil {
		return fmt.Errorf("result is not required for register")
	}
	if parameters != nil {
		return fmt.Errorf("parameters are not required for register")
	}
	return nil
}

func validateCall(procedure, topic string, event, yield, invocation *argsKwargs) error {
	if procedure == "" {
		return fmt.Errorf("procedure is required for call")
	}
	if topic != "" {
		return fmt.Errorf("topic is not required for call")
	}
	if event != nil {
		return fmt.Errorf("event is not required for call")
	}
	if yield != nil {
		return fmt.Errorf("yield is not required for call")
	}
	if invocation != nil {
		return fmt.Errorf("invocation are not required for call")
	}
	return nil
}

func validateSubscribe(topic, procedure string, result, yield, invocation, parameters *argsKwargs) error {
	if topic == "" {
		return fmt.Errorf("topic is required for subscribe")
	}
	if procedure != "" {
		return fmt.Errorf("procedure is not required for subscribe")
	}
	if result != nil {
		return fmt.Errorf("result is not required for subscribe")
	}
	if yield != nil {
		return fmt.Errorf("yield is not required for subscribe")
	}
	if invocation != nil {
		return fmt.Errorf("invocation is not required for subscribe")
	}
	if parameters != nil {
		return fmt.Errorf("parameters are not required for subscribe")
	}
	return nil
}

func validatePublish(topic, procedure string, event, yield, invocation, result *argsKwargs) error {
	if topic == "" {
		return fmt.Errorf("topic is required for publish")
	}
	if procedure != "" {
		return fmt.Errorf("procedure is not required for publish")
	}
	if result != nil {
		return fmt.Errorf("result is not required for publish")
	}
	if yield != nil {
		return fmt.Errorf("yield is not required for publish")
	}
	if invocation != nil {
		return fmt.Errorf("invocation is not required for publish")
	}
	if event != nil {
		return fmt.Errorf("event is not required for publish")
	}
	return nil
}
