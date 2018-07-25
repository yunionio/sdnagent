// Copyright © 2018 Yousong Zhou <zhouyousong@yunionyun.com>
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

package cmd

import (
	"yunion.io/yunion-sdnagent/cmd/sdncli/cli"

	"github.com/spf13/cobra"
)

// delFlowCmd represents the delFlow command
var delFlowCmd = &cobra.Command{
	Use:   "delFlow",
	Short: "Tell sdnagent to delete a flow",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		cli.DoCmd(cmd)
	},
}

func init() {
	rootCmd.AddCommand(delFlowCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// delFlowCmd.PersistentFlags().String("foo", "", "A help for foo")

	cli.InitCmdFlags(delFlowCmd)
}
