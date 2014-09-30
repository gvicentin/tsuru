// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type Plan struct {
	Name     string `bson:"_id" json:"name"`
	Memory   int64  `json:"memory"`
	Swap     int64  `json:"swap"`
	CpuShare int    `json:"cpushare"`
	Default  bool   `json:"default,omitempty"`
}

type PlanValidationError struct{ field string }

func (p PlanValidationError) Error() string {
	return fmt.Sprintf("invalid value for %s", p.field)
}

var (
	ErrPlanNotFound         = errors.New("plan not found")
	ErrPlanAlreadyExists    = errors.New("plan already exists")
	ErrPlanDefaultAmbiguous = errors.New("more than one default plan found")
)

func (plan *Plan) Save() error {
	if plan.Name == "" {
		return PlanValidationError{"name"}
	}
	if plan.CpuShare == 0 {
		return PlanValidationError{"cpushare"}
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if plan.Default {
		_, err := conn.Plans().UpdateAll(bson.M{"default": true}, bson.M{"$unset": bson.M{"default": false}})
		if err != nil {
			return err
		}
	}
	err = conn.Plans().Insert(plan)
	if err != nil && strings.Contains(err.Error(), "duplicate key") {
		return ErrPlanAlreadyExists
	}
	return err
}

func PlansList() ([]Plan, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var plans []Plan
	err = conn.Plans().Find(nil).All(&plans)
	return plans, err
}

func findPlanByName(name string) (*Plan, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var plan Plan
	err = conn.Plans().Find(bson.M{"_id": name}).One(&plan)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, ErrPlanNotFound
		}
		return nil, err
	}
	return &plan, nil
}

func defaultPlan() (*Plan, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var plans []Plan
	err = conn.Plans().Find(bson.M{"default": true}).All(&plans)
	if err != nil {
		return nil, err
	}
	if len(plans) == 0 {
		// For backard compatibility only, this fallback will be removed. You
		// should have at least one plan configured.
		configMemory, _ := config.GetInt("docker:memory")
		configSwap, _ := config.GetInt("docker:swap")
		return &Plan{
			Name:     "autogenerated",
			Memory:   int64(configMemory) * 1024 * 1024,
			Swap:     int64(configSwap-configMemory) * 1024 * 1024,
			CpuShare: 100,
		}, nil
	}
	if len(plans) > 1 {
		return nil, ErrPlanDefaultAmbiguous
	}
	return &plans[0], nil
}

func PlanRemove(planName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Plans().RemoveId(planName)
	if err == mgo.ErrNotFound {
		return ErrPlanNotFound
	}
	return err
}
