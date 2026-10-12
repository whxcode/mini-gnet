package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/user"

	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/logger"
	"github.com/whxcode/mini-gnet/net"
	prompt "github.com/whxcode/mini-gnet/system_propmt"
	"github.com/whxcode/mini-gnet/tools/bash"
	toolsList "github.com/whxcode/mini-gnet/tools/list"
	"github.com/whxcode/mini-gnet/views/message"
)

type bashArguments struct {
	Command string `json:"command"`
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered from panic:", r)
			os.Exit(-1)
		}
	}()

	config := config.LoadConfig()
	if config.ProjectDir == "" {
		config.ProjectDir, _ = os.Getwd()
	}
	tools, err := toolsList.GetToolsList()
	if err != nil {
		logger.Error(err)
		return
	}
	user, _ := user.Current()
	userName := user.Username

	// t, _ := json.MarshalIndent(config, " ", "")
	// logger.Info(string(t))
	//
	// t, _ = json.MarshalIndent(toolsList.GetToolsList(), " ", " ")
	//
	// logger.Info(string(t))

	if err := os.MkdirAll(config.ChatsDir, os.ModePerm); err != nil {
		panic(err)
	}

	var msg *message.TempMessage = message.MakeTempMessage()

	msg.AppendSystemMessage(prompt.CheckAgentPrompt())

	client := &net.Client{
		BaseURL: config.BaseURL,
		ApiKey:  config.APIKey,
		Model:   config.Model,
		HTTP:    &http.Client{},
	}

	// loop cli
	for {
		reader := bufio.NewReader(os.Stdin)
		logger.User("", userName+"说:----------------------")
		query, err := reader.ReadString('\n')
		if err != nil {
			logger.Error(err)
			return
		}
		msg.AppendUserMessage(query)

		for step := 0; step < 8; step++ {
			answer, err := client.Stream(context.Background(), msg.ToChatCompletionMessage(), tools, func(text string) error {
				// logger.Desc(text)
				return nil
			})

			logger.Assistant(answer.Text)

			if err != nil {
				logger.Error(err)
				break
			}

			msg.AppendAssistant(answer.Text, answer.ToolCalls)
			if len(answer.ToolCalls) == 0 {
				fmt.Println()
				break
			}

			for _, call := range answer.ToolCalls {
				if call.Function.Name != "run_bash" {
					msg.AppendTool(call.ID, "unknown tool: "+call.Function.Name)
					continue
				}
				var args bashArguments
				if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
					msg.AppendTool(call.ID, "invalid tool arguments: "+err.Error())
					continue
				}

				fmt.Println(args)

				logger.Info("执行: ", args.Command)
				output, err := bash.Run(context.Background(), config.ProjectDir, args.Command)
				if err != nil {
					output += "\n" + err.Error()
				}

				msg.AppendTool(call.ID, output)
			}
		}
	}
}
