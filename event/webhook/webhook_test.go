// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	service *webhookService
}

var _ = check.Suite(&S{})

func (s *S) SetUpTest(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=150")
	config.Set("database:name", "tsuru_event_webhook_tests")

	storagev2.Reset()

	err := storagev2.ClearAllCollections(nil)
	c.Assert(err, check.IsNil)
	svc, err := WebhookService()
	c.Assert(err, check.IsNil)
	s.service = svc.(*webhookService)
	servicemock.SetMockService(&servicemock.MockService{})
}

func (s *S) TearDownTest(c *check.C) {
	err := s.service.Shutdown(context.Background())
	c.Assert(err, check.IsNil)
}

func (s *S) TestWebhookServiceNotify(c *check.C) {
	evt, err := event.New(context.TODO(), &event.Opts{
		Target: eventTypes.Target{Type: "app", Value: "myapp"},
		ExtraTargets: []eventTypes.ExtraTarget{
			{Target: eventTypes.Target{Type: "app", Value: "xapp1"}},
			{Target: eventTypes.Target{Type: "app", Value: "xapp2"}},
		},
		RawOwner: eventTypes.Owner{
			Type: "user",
			Name: "me@me.com",
		},
		Kind:    permission.PermAppUpdateEnvSet,
		Allowed: event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxApp, "myapp")),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	called := make(chan struct{})
	var receivedReq *http.Request
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(called)
		receivedBody, _ = io.ReadAll(r.Body)
		receivedReq = r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	err = s.service.storage.Insert(context.TODO(), eventTypes.Webhook{
		Name:   "xyz",
		URL:    srv.URL + "/a/b/c?a=b&c=d",
		Method: "PUT",
		Body:   "ahoy {{ --",
		Headers: http.Header{
			"X-Ahoy": []string{"Errrr"},
		},
	})
	c.Assert(err, check.IsNil)
	s.service.Notify(context.TODO(), evt.UniqueID.Hex())
	<-called
	c.Assert(string(receivedBody), check.Equals, "ahoy {{ --")
	c.Assert(receivedReq.Method, check.Equals, "PUT")
	c.Assert(receivedReq.URL.Path, check.Equals, "/a/b/c")
	c.Assert(receivedReq.URL.Query(), check.DeepEquals, url.Values{
		"a": []string{"b"},
		"c": []string{"d"},
	})
	c.Assert(receivedReq.Header, check.DeepEquals, http.Header{
		"X-Ahoy":          []string{"Errrr"},
		"User-Agent":      []string{"tsuru-webhook-client/1.0"},
		"Accept-Encoding": []string{"gzip"},
		"Content-Length":  []string{"10"},
	})
}

func (s *S) TestWebhookServiceNotifyDefaultBody(c *check.C) {
	evt, err := event.New(context.TODO(), &event.Opts{
		Target: eventTypes.Target{Type: "app", Value: "myapp"},
		ExtraTargets: []eventTypes.ExtraTarget{
			{Target: eventTypes.Target{Type: "app", Value: "xapp1"}},
			{Target: eventTypes.Target{Type: "app", Value: "xapp2"}},
		},
		RawOwner: eventTypes.Owner{
			Type: "user",
			Name: "me@me.com",
		},
		Kind:    permission.PermAppUpdateEnvSet,
		Allowed: event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxApp, "myapp")),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	doneEvt, err := event.GetByID(context.TODO(), evt.UniqueID)
	c.Assert(err, check.IsNil)
	evtData, err := json.Marshal(doneEvt)
	c.Assert(err, check.IsNil)
	called := make(chan struct{})
	var receivedReq *http.Request
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(called)
		receivedBody, _ = io.ReadAll(r.Body)
		receivedReq = r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	err = s.service.storage.Insert(context.TODO(), eventTypes.Webhook{
		Name: "xyz",
		URL:  srv.URL,
	})
	c.Assert(err, check.IsNil)
	s.service.Notify(context.TODO(), evt.UniqueID.Hex())
	<-called
	c.Assert(string(receivedBody), check.Equals, string(evtData))
	c.Assert(receivedReq.Method, check.Equals, "POST")
	c.Assert(receivedReq.URL.Path, check.Equals, "/")
	c.Assert(receivedReq.Header.Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestWebhookServiceNotifyTemplate(c *check.C) {
	evt, err := event.New(context.TODO(), &event.Opts{
		Target: eventTypes.Target{Type: "app", Value: "myapp"},
		ExtraTargets: []eventTypes.ExtraTarget{
			{Target: eventTypes.Target{Type: "app", Value: "xapp1"}},
			{Target: eventTypes.Target{Type: "app", Value: "xapp2"}},
		},
		RawOwner: eventTypes.Owner{
			Type: "user",
			Name: "me@me.com",
		},
		Kind:    permission.PermAppUpdateEnvSet,
		Allowed: event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxApp, "myapp")),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	called := make(chan struct{})
	var receivedReq *http.Request
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(called)
		receivedBody, _ = io.ReadAll(r.Body)
		receivedReq = r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	err = s.service.storage.Insert(context.TODO(), eventTypes.Webhook{
		Name:   "xyz",
		URL:    srv.URL + "/a/b/c?a=b&c=d",
		Method: "PUT",
		Body:   "{{.Kind.Name}} event for {{.Target.Type}} named {{.Target.Value}}",
		Headers: http.Header{
			"X-Ahoy": []string{"Errrr"},
		},
	})
	c.Assert(err, check.IsNil)
	s.service.Notify(context.TODO(), evt.UniqueID.Hex())
	<-called
	c.Assert(string(receivedBody), check.Equals, "app.update.env.set event for app named myapp")
	c.Assert(receivedReq.Method, check.Equals, "PUT")
	c.Assert(receivedReq.URL.Path, check.Equals, "/a/b/c")
	c.Assert(receivedReq.URL.Query(), check.DeepEquals, url.Values{
		"a": []string{"b"},
		"c": []string{"d"},
	})
	c.Assert(receivedReq.Header, check.DeepEquals, http.Header{
		"X-Ahoy":          []string{"Errrr"},
		"User-Agent":      []string{"tsuru-webhook-client/1.0"},
		"Accept-Encoding": []string{"gzip"},
		"Content-Length":  []string{"44"},
	})
}

func (s *S) TestWebhookServiceNotifyProxy(c *check.C) {
	evt, err := event.New(context.TODO(), &event.Opts{
		Target: eventTypes.Target{Type: "app", Value: "myapp"},
		ExtraTargets: []eventTypes.ExtraTarget{
			{Target: eventTypes.Target{Type: "app", Value: "xapp1"}},
			{Target: eventTypes.Target{Type: "app", Value: "xapp2"}},
		},
		RawOwner: eventTypes.Owner{
			Type: "user",
			Name: "me@me.com",
		},
		Kind:    permission.PermAppUpdateEnvSet,
		Allowed: event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxApp, "myapp")),
	})
	c.Assert(err, check.IsNil)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	doneEvt, err := event.GetByID(context.TODO(), evt.UniqueID)
	c.Assert(err, check.IsNil)
	evtData, err := json.Marshal(doneEvt)
	c.Assert(err, check.IsNil)
	called := make(chan struct{})
	var receivedReq *http.Request
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(called)
		receivedBody, _ = io.ReadAll(r.Body)
		receivedReq = r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	err = s.service.storage.Insert(context.TODO(), eventTypes.Webhook{
		Name:     "xyz",
		URL:      "http://xyz/",
		ProxyURL: srv.URL,
	})
	c.Assert(err, check.IsNil)
	s.service.Notify(context.TODO(), evt.UniqueID.Hex())
	<-called
	c.Assert(string(receivedBody), check.Equals, string(evtData))
	c.Assert(receivedReq.Method, check.Equals, "POST")
	c.Assert(receivedReq.RequestURI, check.Equals, "http://xyz/")
	c.Assert(receivedReq.Header.Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestWebhookServiceCreate(c *check.C) {
	err := s.service.Create(context.TODO(), eventTypes.Webhook{
		Name: "xyz",
		URL:  "http://a",
		Body: "ahoy",
		Headers: http.Header{
			"X-Ahoy": []string{"Errrr"},
		},
		EventFilter: eventTypes.WebhookEventFilter{
			TargetTypes: []string{"app"},
		},
	})
	c.Assert(err, check.IsNil)
	w, err := s.service.Find(context.TODO(), "xyz")
	c.Assert(err, check.IsNil)
	c.Assert(w, check.DeepEquals, eventTypes.Webhook{
		Name: "xyz",
		URL:  "http://a",
		Body: "ahoy",
		Headers: http.Header{
			"X-Ahoy": []string{"Errrr"},
		},
		EventFilter: eventTypes.WebhookEventFilter{
			TargetTypes:  []string{"app"},
			TargetValues: []string{},
			KindTypes:    []string{},
			KindNames:    []string{},
		},
	})
}

func (s *S) TestWebhookServiceCreateInvalid(c *check.C) {
	var tests = []struct {
		name, url, proxyURL string
		expectedErr         string
	}{
		{
			name:        "",
			url:         "http://a",
			expectedErr: "webhook name must not be empty",
		},
		{
			name: "_-*x",
			url:  "http://a",
			expectedErr: "Invalid webhook name, webhook name should have at most 40 " +
				"characters, containing only lower case letters, numbers or dashes, " +
				"starting with a letter.",
		},
		{
			name: "c",
			url:  "http://a",
		},
		{
			name:        "c",
			url:         "http://a",
			expectedErr: eventTypes.ErrWebhookAlreadyExists.Error(),
		},
		{
			name:        "d",
			expectedErr: "webhook url must not be empty",
		},
		{
			name:        "d",
			url:         ":/:x",
			expectedErr: "webhook url is not valid: parse .*: missing protocol scheme",
		},
		{
			name:        "d",
			url:         "http://valid",
			proxyURL:    ":/:x",
			expectedErr: "webhook proxy url is not valid: parse .*: missing protocol scheme",
		},
	}

	for _, test := range tests {
		err := s.service.Create(context.TODO(), eventTypes.Webhook{
			Name:     test.name,
			URL:      test.url,
			ProxyURL: test.proxyURL,
		})
		if test.expectedErr == "" {
			c.Check(err, check.IsNil)
		} else {
			c.Check(err, check.ErrorMatches, test.expectedErr)
		}
	}
}

func (s *S) TestWebhookServiceUpdate(c *check.C) {
	err := s.service.Create(context.TODO(), eventTypes.Webhook{
		Name: "xyz",
		URL:  "http://a",
	})
	c.Assert(err, check.IsNil)
	err = s.service.Update(context.TODO(), eventTypes.Webhook{
		Name: "xyz",
		URL:  "http://b",
	})
	c.Assert(err, check.IsNil)
	w, err := s.service.Find(context.TODO(), "xyz")
	c.Assert(err, check.IsNil)
	c.Assert(w, check.DeepEquals, eventTypes.Webhook{
		Name:    "xyz",
		URL:     "http://b",
		Headers: http.Header{},
		EventFilter: eventTypes.WebhookEventFilter{
			TargetTypes:  []string{},
			TargetValues: []string{},
			KindTypes:    []string{},
			KindNames:    []string{},
		},
	})
}

func (s *S) TestWebhookServiceUpdateInvalid(c *check.C) {
	err := s.service.Update(context.TODO(), eventTypes.Webhook{
		Name: "xyz",
		URL:  "http://b",
	})
	c.Assert(err, check.Equals, eventTypes.ErrWebhookNotFound)
}

func (s *S) TestWebhookServiceDelete(c *check.C) {
	err := s.service.Create(context.TODO(), eventTypes.Webhook{
		Name: "xyz",
		URL:  "http://a",
	})
	c.Assert(err, check.IsNil)
	err = s.service.Delete(context.TODO(), "xyz")
	c.Assert(err, check.IsNil)
	_, err = s.service.Find(context.TODO(), "xyz")
	c.Assert(err, check.Equals, eventTypes.ErrWebhookNotFound)
}

func (s *S) TestWebhookServiceDeleteNotFound(c *check.C) {
	err := s.service.Delete(context.TODO(), "xyz")
	c.Assert(err, check.Equals, eventTypes.ErrWebhookNotFound)
}
