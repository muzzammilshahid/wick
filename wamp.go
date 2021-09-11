package main

import (
	"context"
	"fmt"
	client2 "github.com/gammazero/nexus/client"
	"github.com/gammazero/nexus/v3/client"
	"github.com/gammazero/nexus/v3/wamp"
	"log"
	"os"
	"os/signal"
)

func subscribe(URLSub string, realmSub string, topicSub string){
	logger := log.New(os.Stdout, "Subscriber> ", 0)
	cfg := client.Config{
		Realm:  realmSub,
		Logger: logger,
	}

	// Connect subscriber session.
	subscriber, err := client.ConnectNet(context.Background(), URLSub, cfg)
	if err != nil {
		logger.Fatal(err)
	}
	defer func(subscriber *client2.Client) {
		err := subscriber.Close()
		if err != nil {
			println(err)
		}
	}(subscriber)

	// Define function to handle events received.
	eventHandler := func(event *wamp.Event) {
		logger.Println("Received ", topicSub, " event")
		if len(event.Arguments) != 0 {
			logger.Println("  Event Message:", event.Arguments[0])
		}
	}

	// Subscribe to topic.
	err = subscriber.Subscribe(topicSub, eventHandler, nil)
	if err != nil {
		logger.Fatal("subscribe error:", err)
	}
	logger.Println("Subscribed to", topicSub)

	// Wait for CTRL-c or client close while handling events.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	select {
	case <-sigChan:
	case <-subscriber.Done():
		logger.Print("Router gone, exiting")
		return // router gone, just exit
	}

	// Unsubscribe from topic.
	if err = subscriber.Unsubscribe(topicSub); err != nil {
		logger.Println("Failed to unsubscribe:", err)
	}

}

func publish(URLPub string, realmPub string, topicPub string, argsList wamp.List) {
	logger := log.New(os.Stdout, "Publisher> ", 0)
	cfg := client.Config{
		Realm:  realmPub,
		Logger: logger,
	}

	// Connect publisher session.
	publisher, err := client.ConnectNet(context.Background(), URLPub, cfg)
	if err != nil {
		logger.Fatal(err)
	}
	defer func(publisher *client2.Client) {
		err := publisher.Close()
		if err != nil {
			println(err)
		}
	}(publisher)

	// Publish to topic.
	err = publisher.Publish(topicPub, nil,argsList, nil)
	if err != nil {
		logger.Fatal("publish error:", err)
	}
	logger.Println("Published", topicPub, "event")
}

func register(URLReg string, realmReg string, procedureReg string){
	logger := log.New(os.Stdout, "Register> ", 0)
	cfg := client.Config{
		Realm:  realmReg,
		Logger: logger,
	}
	register, err := client.ConnectNet(context.Background(), URLReg, cfg)
	logger.Println("Connected to ", URLReg)
	if err != nil {
		logger.Fatal(err)
	}
	defer func(register *client2.Client) {
		err := register.Close()
		if err != nil {
			println(err)
		}
	}(register)

	eventHandler:= func(ctx context.Context, inv *wamp.Invocation) client.InvokeResult {
		fmt.Println(inv.Arguments,inv.ArgumentsKw)
		return client.InvokeResult{Args: wamp.List{inv.Arguments}}
	}

	if err = register.Register(procedureReg, eventHandler, nil);
		err != nil {
		logger.Fatal("Failed to publish procedure:", err)
	}
	logger.Println("Registered procedure", procedureReg, "with router")

	// Wait for CTRL-c or client close while handling remote procedure calls.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	select {
	case <-sigChan:
	case <-register.Done():
		logger.Print("Router gone, exiting")
		return // router gone, just exit
	}

	if err = register.Unregister(procedureReg); err != nil {
		logger.Println("Failed to unregister procedure:", err)
	}

	logger.Println("Registered procedure with router")

}

func call(URLCal string, realmCal string, procedureCal string, argsList wamp.List){
	logger := log.New(os.Stderr, "Caller> ", 0)


	cfg := client.Config{
		Realm:  realmCal,
		Logger: logger,
	}

	// Connect caller client
	caller, err := client.ConnectNet(context.Background(), URLCal, cfg)
	if err != nil {
		logger.Fatal(err)
	}
	defer func(caller *client2.Client) {
		err := caller.Close()
		if err != nil {
			println(err)
		}
	}(caller)

	ctx := context.Background()

	result, err := caller.Call(ctx, procedureCal, nil, argsList, nil, nil)
	if err != nil {
		logger.Fatal(err)
	}
	for i := range result.Arguments {
		locTime, _ := wamp.AsString(result.Arguments[i])
		logger.Println(locTime)
	}
}