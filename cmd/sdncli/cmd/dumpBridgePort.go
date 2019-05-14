// Copyright 2019 Yunion
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
	"github.com/spf13/cobra"

	"yunion.io/x/sdnagent/cmd/sdncli/cli"
)

// dumpBridgePortCmd represents the dumpBridgePort command
var dumpBridgePortCmd = &cobra.Command{
	Use:   "dumpBridgePort",
	Short: "Dump OpenFlow port number",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		cli.DoCmd(cmd)
	},
}

func init() {
	rootCmd.AddCommand(dumpBridgePortCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// dumpBridgePortCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// dumpBridgePortCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	cli.InitCmdFlags(dumpBridgePortCmd)
}
