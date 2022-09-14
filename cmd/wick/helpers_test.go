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
	"testing"

	"github.com/gammazero/nexus/v3/transport/serialize"
	"github.com/stretchr/testify/assert"
)

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
