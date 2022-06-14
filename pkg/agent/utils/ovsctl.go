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

package utils

import (
	"context"
	"os/exec"

	"yunion.io/x/pkg/errors"
)

func RunOvsctl(ctx context.Context, args []string) error {
	_, err := ExecOvsctl(ctx, args)
	return err
}

func ExecOvsctl(ctx context.Context, args []string) ([]byte, error) {
	if len(args) == 0 {
		panic("exec: empty args")
	}
	tos := func(args []string) string {
		s := ""
		for _, arg := range args {
			if arg != "--" {
				s += " " + arg
			} else {
				s += " \\\n  " + arg
			}
		}
		return s
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.Output()
	if err != nil {
		s := tos(args)
		err = errors.Wrap(err, s)
		return nil, err
	}
	return output, nil
}
