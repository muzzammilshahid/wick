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

package core

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gammazero/nexus/v3/client"
	"github.com/gammazero/nexus/v3/transport/serialize"
	"github.com/gammazero/nexus/v3/wamp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ed25519"
)

type ClientInfo struct {
	Url        string
	Realm      string
	Serializer serialize.Serialization
	Authid     string
	Authrole   string
	AuthMethod string
	PrivateKey string
	Ticket     string
	Secret     string
}

func listToWampList(args []string) wamp.List {
	var arguments wamp.List

	if args == nil {
		return wamp.List{}
	}

	for _, value := range args {

		var mapJson map[string]interface{}
		var mapList []map[string]interface{}
		var simpleList []interface{}

		if number, errNumber := strconv.Atoi(value); errNumber == nil {
			arguments = append(arguments, number)
		} else if float, errFloat := strconv.ParseFloat(value, 64); errFloat == nil {
			arguments = append(arguments, float)
		} else if boolean, errBoolean := strconv.ParseBool(value); errBoolean == nil {
			arguments = append(arguments, boolean)
		} else if errJson := json.Unmarshal([]byte(value), &mapJson); errJson == nil {
			arguments = append(arguments, mapJson)
		} else if errMapList := json.Unmarshal([]byte(value), &mapList); errMapList == nil {
			arguments = append(arguments, mapList)
		} else if errList := json.Unmarshal([]byte(value), &simpleList); errList == nil {
			arguments = append(arguments, simpleList)
		} else {
			arguments = append(arguments, value)
		}
	}

	return arguments
}

func dictToWampDict(kwargs map[string]string) wamp.Dict {
	var keywordArguments wamp.Dict = make(map[string]interface{})

	for key, value := range kwargs {

		var mapJson map[string]interface{}
		var mapList []map[string]interface{}
		var simpleList []interface{}

		if number, errNumber := strconv.Atoi(value); errNumber == nil {
			keywordArguments[key] = number
		} else if float, errFloat := strconv.ParseFloat(value, 64); errFloat == nil {
			keywordArguments[key] = float
		} else if boolean, errBoolean := strconv.ParseBool(value); errBoolean == nil {
			keywordArguments[key] = boolean
		} else if errJson := json.Unmarshal([]byte(value), &mapJson); errJson == nil {
			keywordArguments[key] = mapJson
		} else if errMapList := json.Unmarshal([]byte(value), &mapList); errMapList == nil {
			keywordArguments[key] = mapList
		} else if errList := json.Unmarshal([]byte(value), &simpleList); errList == nil {
			keywordArguments[key] = simpleList
		} else {
			keywordArguments[key] = value
		}
	}
	return keywordArguments
}

func registerInvocationHandler(session *client.Client, procedure string, command string,
	invokeCount int, hasMaxInvokeCount bool) func(ctx context.Context, inv *wamp.Invocation) client.InvokeResult {
	invocationHandler := func(ctx context.Context, inv *wamp.Invocation) client.InvokeResult {
		output, err := argsKWArgs(inv.Arguments, inv.ArgumentsKw, nil)
		if err != nil {
			return client.InvokeResult{Err: "wamp.error.internal_error", Args: wamp.List{err}}
		}
		fmt.Println(output)

		result := ""
		if command != "" {
			out, _, err := shellOut(command)
			if err != nil {
				log.Println("error: ", err)
			}
			result = out
		}

		if hasMaxInvokeCount {
			invokeCount--
			if invokeCount == 0 {
				session.Unregister(procedure)
				time.AfterFunc(1*time.Second, func() {
					log.Println("session closing")
					session.Close()
				})
			}
		}

		return client.InvokeResult{Args: wamp.List{result}}
	}
	return invocationHandler
}

func argsKWArgs(args wamp.List, kwArgs wamp.Dict, details wamp.Dict) (string, error) {
	var outputString string
	if details != nil {
		jsonString, err := json.MarshalIndent(details, "", "    ")
		if err != nil {
			return "", err
		}
		outputString = fmt.Sprintf("details:%s\n", jsonString)
	}
	if len(args) != 0 {
		jsonString, err := json.MarshalIndent(args, "", "    ")
		if err != nil {
			return "", err
		}
		outputString = fmt.Sprintf("%sargs:\n%s", outputString, jsonString)
	}

	if len(kwArgs) != 0 {
		jsonString, err := json.MarshalIndent(kwArgs, "", "    ")
		if err != nil {
			return "", err
		}
		outputString = fmt.Sprintf("%skwargs:\n%s", outputString, jsonString)
	}

	if len(args) == 0 && len(kwArgs) == 0 && details == nil {
		outputString = fmt.Sprintf("args: []\nkwargs: {}")
	}
	return outputString, nil
}

func progressArgsKWArgs(args wamp.List, kwArgs wamp.Dict) (string, error) {
	var outputString string
	if len(args) != 0 {
		jsonString, err := json.Marshal(args)
		if err != nil {
			return "", err
		}
		outputString = fmt.Sprintf("args: %s  ", jsonString)
	}

	if len(kwArgs) != 0 {
		bs, err := json.Marshal(kwArgs)
		if err != nil {
			return "", err
		}
		outputString = fmt.Sprintf("%skwargs: %s", outputString, bs)
	}

	if len(args) == 0 && len(kwArgs) == 0 {
		outputString = fmt.Sprintf("args: [] kwargs: {}")
	}

	outputString = fmt.Sprintf("%s\n", outputString)

	return outputString, nil
}

func shellOut(command string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var cmd *exec.Cmd
	cmd = exec.Command("sh", "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func buildStringFromMap(brokerFeatures map[string]interface{}) string {
	var builder strings.Builder
	for key, value := range brokerFeatures {
		fmt.Fprintf(&builder, "%s=%v, ", key, value)
	}
	return strings.TrimRight(builder.String(), ", ")
}

func getKeyPair(privateKeyKex string) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	privateKeyRaw, err := hex.DecodeString(privateKeyKex)
	if err != nil {
		return nil, nil, err
	}
	var privateKey ed25519.PrivateKey

	if len(privateKeyRaw) == 32 {
		privateKey = ed25519.NewKeyFromSeed(privateKeyRaw)
	} else if len(privateKeyRaw) == 64 {
		privateKey = ed25519.NewKeyFromSeed(privateKeyRaw[:32])
	} else {
		return nil, nil,
			fmt.Errorf("invalid private key: Cryptosign private key must be either 32 or 64 characters long")
	}

	publicKey := privateKey.Public().(ed25519.PublicKey)

	return publicKey, privateKey, nil
}

func sanitizeURL(url string) string {
	if strings.HasPrefix(url, "rss") {
		return "tcp" + strings.TrimPrefix(url, "rss")
	} else if strings.HasPrefix(url, "rs") {
		return "tcp" + strings.TrimPrefix(url, "rs")
	}
	return url
}

func getErrorFromErrorChannel(resC chan error) error {
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
