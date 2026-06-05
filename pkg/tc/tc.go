// Copyright 2019 Yunion
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tc

import (
	"context"
	"os/exec"
	"strings"

	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"
)

type TcCli struct {
	force   bool
	details bool
}

func (tc *TcCli) Force(force bool) *TcCli {
	tc.force = force
	return tc
}

func (tc *TcCli) Details(details bool) *TcCli {
	tc.details = details
	return tc
}

func (tc *TcCli) QdiscShow(ctx context.Context, ifname string) (*QdiscTree, error) {
	outputs := make([]string, 3)
	for i, tcType := range []string{"qdisc", "class", "filter"} {
		cmd := exec.CommandContext(ctx, "tc", tcType, "show", "dev", ifname)
		output, err := cmd.Output()
		if err != nil {
			return nil, errors.Wrapf(err, "tc %s show", tcType)
		}
		outputs[i] = string(output)
	}
	{
		// show filter root
		cmd := exec.CommandContext(ctx, "tc", "filter", "show", "dev", ifname, "root")
		output, err := cmd.Output()
		if err != nil {
			return nil, errors.Wrapf(err, "tc filter show root")
		}
		outputs[2] += string(output)
	}
	qt, err := NewQdiscTreeFromString(string(outputs[0]), string(outputs[1]), string(outputs[2]))
	return qt, err
}

func (tc *TcCli) Batch(ctx context.Context, cmdlines [][]string) (string, string, error) {
	var errs []error
	var stdout strings.Builder
	var stderr strings.Builder
	var err error
	for i := range cmdlines {
		cmdline := cmdlines[i]
		sout, serr, e := tc.singleCmd(ctx, cmdline)
		if len(sout) > 0 {
			stdout.WriteString(sout)
		}
		if len(serr) > 0 {
			stderr.WriteString(serr)
		}
		if e != nil {
			errs = append(errs, e)
		}
	}
	if len(errs) > 0 {
		err = errors.NewAggregate(errs)
	}
	return stdout.String(), stderr.String(), err
}

func (tc *TcCli) singleCmd(ctx context.Context, cmdline []string) (stdout string, stderr string, err error) {
	args := make([]string, 0, 4)
	if tc.details {
		args = append(args, "-details")
	}
	if tc.force {
		args = append(args, "-force")
	}
	args = append(args, cmdline...)
	cmd := exec.CommandContext(ctx, "tc", args...)
	output, err := cmd.Output()
	if err == nil {
		stdout = string(output)
	} else {
		log.Errorf("tc: %s failed: %s", strings.Join(cmdline, " "), err)
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
	}
	return stdout, stderr, err
}

func NewTcCli() *TcCli {
	return &TcCli{}
}
