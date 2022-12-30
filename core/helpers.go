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
	"os"
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

const (
	sixtyFourInt = 64
	thirtyTwoInt = 32
)

func getPathIfFile(arg string) (path string, isFile bool) {
	split := strings.SplitN(arg, ":=", 2) //nolint:gomnd
	if len(split) == 2 {                  //nolint:gomnd
		return split[1], true
	}
	return "", false
}

func listToWampList(args []string, checkFile bool) (wamp.List, error) {
	var arguments wamp.List

	if args == nil {
		return wamp.List{}, nil
	}

	for _, value := range args {
		var mapJson map[string]interface{}
		var mapList []map[string]interface{}
		var simpleList []interface{}

		switch {
		case strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`):
			arguments = append(arguments, value[1:len(value)-1])
		case strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`):
			arguments = append(arguments, value[1:len(value)-1])
		default:
			if checkFile {
				filePath, isFile := getPathIfFile(value)
				if isFile {
					fileBytes, err := os.ReadFile(filePath)
					if err != nil {
						return nil, err
					}
					arguments = append(arguments, fileBytes)
					continue
				}
			}
			if number, errNumber := strconv.Atoi(value); errNumber == nil {
				arguments = append(arguments, number)
			} else if float, errFloat := strconv.ParseFloat(value, sixtyFourInt); errFloat == nil {
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
	}

	return arguments, nil
}

func dictToWampDict(kwargs map[string]string, checkFile bool) (wamp.Dict, error) {
	var keywordArguments wamp.Dict = make(map[string]interface{})

	for key, value := range kwargs {
		var mapJson map[string]interface{}
		var mapList []map[string]interface{}
		var simpleList []interface{}

		switch {
		case strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`):
			keywordArguments[key] = value[1 : len(value)-1]
		case strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`):
			keywordArguments[key] = value[1 : len(value)-1]
		default:
			if checkFile {
				filePath, isFile := getPathIfFile(value)
				if isFile {
					fileBytes, err := os.ReadFile(filePath)
					if err != nil {
						return nil, err
					}
					keywordArguments[key] = fileBytes
					continue
				}
			}
			if number, errNumber := strconv.Atoi(value); errNumber == nil {
				keywordArguments[key] = number
			} else if float, errFloat := strconv.ParseFloat(value, sixtyFourInt); errFloat == nil {
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
	}
	return keywordArguments, nil
}

func registerInvocationHandler(session *client.Client, procedure string, command string,
	invokeCount int, hasMaxInvokeCount bool) func(ctx context.Context, inv *wamp.Invocation) client.InvokeResult {
	invocationHandler := func(ctx context.Context, inv *wamp.Invocation) client.InvokeResult {
		output, err := ArgsKWArgs(inv.Arguments, inv.ArgumentsKw, nil)
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
				_ = session.Unregister(procedure)
				time.AfterFunc(1*time.Second, func() {
					log.Println("session closing")
					_ = session.Close()
				})
			}
		}

		return client.InvokeResult{Args: wamp.List{result}}
	}
	return invocationHandler
}

func ArgsKWArgs(args wamp.List, kwArgs wamp.Dict, details wamp.Dict) (string, error) {
	var builder strings.Builder
	if details != nil {
		jsonString, err := json.MarshalIndent(details, "", "    ")
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&builder, "details:%s\n", jsonString)
	}
	if len(args) != 0 {
		jsonString, err := json.MarshalIndent(args, "", "    ")
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&builder, "args:\n%s", jsonString)
	}

	if len(kwArgs) != 0 {
		jsonString, err := json.MarshalIndent(kwArgs, "", "    ")
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&builder, "kwargs:\n%s", jsonString)
	}

	if len(args) == 0 && len(kwArgs) == 0 && details == nil {
		fmt.Fprintf(&builder, "args: []\nkwargs: {}")
	}
	return builder.String(), nil
}

func progressArgsKWArgs(args wamp.List, kwArgs wamp.Dict) (string, error) {
	var builder strings.Builder
	if len(args) != 0 {
		jsonString, err := json.Marshal(args)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&builder, "args: %s", jsonString)
	}

	if len(kwArgs) != 0 {
		bs, err := json.Marshal(kwArgs)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&builder, "kwargs: %s", bs)
	}

	if len(args) == 0 && len(kwArgs) == 0 {
		fmt.Fprintf(&builder, "args: [] kwargs: {}")
	}

	return builder.String(), nil
}

func shellOut(command string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func getKeyPair(privateKeyKex string) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	privateKeyRaw, err := hex.DecodeString(privateKeyKex)
	if err != nil {
		return nil, nil, err
	}
	var privateKey ed25519.PrivateKey

	if len(privateKeyRaw) == thirtyTwoInt {
		privateKey = ed25519.NewKeyFromSeed(privateKeyRaw)
	} else if len(privateKeyRaw) == sixtyFourInt {
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

func encodeToJson(value interface{}) (string, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "    ")
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return buffer.String(), nil
}
