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

package modules

import "yunion.io/x/onecloud/pkg/mcclient/modulebase"

var (
	BillDetails modulebase.ResourceManager
)

func init() {
	BillDetails = NewMeterManager("bill_detail", "bill_details",
		[]string{"bill_id", "account", "platform", "region", "manager_project", "res_id",
			"res_type", "res_name", "start_time", "end_time", "charge_type", "item_rate", "item_fee"},
		[]string{},
	)
	register(&BillDetails)
}
