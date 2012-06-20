package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

type AppRun struct{}

func (c *AppRun) Info() *Info {
	desc := `run a command in all instances of the app, and prints the output.
Notice that you may need quotes to run your command if you want to deal with
input and outputs redirects, and pipes.
`
	return &Info{
		Name:    "run",
		Usage:   `run appname command commandarg1 commandarg2 ... commandargn`,
		Desc:    desc,
		MinArgs: 1,
	}
}

func (c *AppRun) Run(context *Context, client Doer) error {
	appName := context.Args[0]
	url := GetUrl(fmt.Sprintf("/apps/%s/run", appName))
	b := strings.NewReader(strings.Join(context.Args[1:], " "))
	request, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}
	r, err := client.Do(request)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	_, err = io.Copy(context.Stdout, r.Body)
	return err
}
