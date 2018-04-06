// Copyright 2018 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rpc

import (
	"fmt"
	"testing"

	"golang.org/x/net/context"

	"github.com/DATA-DOG/go-sqlmock"

	"go.chromium.org/luci/machine-db/api/common/v1"
	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestListKVMs(t *testing.T) {
	Convey("listKVMs", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		columns := []string{"h.name", "h.vlan_id", "p.name", "r.name", "d.name", "k.description", "k.mac_address", "i.ipv4", "k.state"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, p.name, r.name, d.name, k.description, k.mac_address, i.ipv4, k.state
				FROM kvms k, hostnames h, platforms p, racks r, datacenters d, ips i
				WHERE k.hostname_id = h.id AND k.platform_id = p.id AND k.rack_id = r.id AND r.datacenter_id = d.id AND i.hostname_id = h.id
					AND h.name IN \(\?\)$
			`
			req := &crimson.ListKVMsRequest{
				Names: []string{"kvm"},
			}
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			KVMs, err := listKVMs(c, req)
			So(err, ShouldErrLike, "failed to fetch KVMs")
			So(KVMs, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty request", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, p.name, r.name, d.name, k.description, k.mac_address, i.ipv4, k.state
				FROM kvms k, hostnames h, platforms p, racks r, datacenters d, ips i
				WHERE k.hostname_id = h.id AND k.platform_id = p.id AND k.rack_id = r.id AND r.datacenter_id = d.id AND i.hostname_id = h.id$
			`
			req := &crimson.ListKVMsRequest{}
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			KVMs, err := listKVMs(c, req)
			So(err, ShouldBeNil)
			So(KVMs, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty response", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, p.name, r.name, d.name, k.description, k.mac_address, i.ipv4, k.state
				FROM kvms k, hostnames h, platforms p, racks r, datacenters d, ips i
				WHERE k.hostname_id = h.id AND k.platform_id = p.id AND k.rack_id = r.id AND r.datacenter_id = d.id AND i.hostname_id = h.id
					AND h.name IN \(\?\)$
			`
			req := &crimson.ListKVMsRequest{
				Names: []string{"kvm"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0]).WillReturnRows(rows)
			KVMs, err := listKVMs(c, req)
			So(err, ShouldBeNil)
			So(KVMs, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("non-empty", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, p.name, r.name, d.name, k.description, k.mac_address, i.ipv4, k.state
				FROM kvms k, hostnames h, platforms p, racks r, datacenters d, ips i
				WHERE k.hostname_id = h.id AND k.platform_id = p.id AND k.rack_id = r.id AND r.datacenter_id = d.id AND i.hostname_id = h.id
					AND h.name IN \(\?,\?\)$
			`
			req := &crimson.ListKVMsRequest{
				Names: []string{"kvm 1", "kvm 2"},
			}
			rows.AddRow("kvm 1", 1, "platform 1", "rack 1", "datacenter 1", "", 1, 1, common.State_FREE)
			rows.AddRow("kvm 2", 2, "platform 2", "rack 2", "datacenter 2", "", 2, 2, common.State_SERVING)
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0], req.Names[1]).WillReturnRows(rows)
			KVMs, err := listKVMs(c, req)
			So(err, ShouldBeNil)
			So(KVMs, ShouldResemble, []*crimson.KVM{
				{
					Name:       "kvm 1",
					Vlan:       1,
					Platform:   "platform 1",
					Rack:       "rack 1",
					Datacenter: "datacenter 1",
					MacAddress: "00:00:00:00:00:01",
					Ipv4:       "0.0.0.1",
					State:      common.State_FREE,
				},
				{
					Name:       "kvm 2",
					Vlan:       2,
					Platform:   "platform 2",
					Rack:       "rack 2",
					Datacenter: "datacenter 2",
					MacAddress: "00:00:00:00:00:02",
					Ipv4:       "0.0.0.2",
					State:      common.State_SERVING,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, p.name, r.name, d.name, k.description, k.mac_address, i.ipv4, k.state
				FROM kvms k, hostnames h, platforms p, racks r, datacenters d, ips i
				WHERE k.hostname_id = h.id AND k.platform_id = p.id AND k.rack_id = r.id AND r.datacenter_id = d.id AND i.hostname_id = h.id$
			`
			req := &crimson.ListKVMsRequest{}
			rows.AddRow("kvm 1", 1, "platform 1", "rack 1", "datacenter 1", "", 1, 1, common.State_FREE)
			rows.AddRow("kvm 2", 2, "platform 2", "rack 2", "datacenter 2", "", 2, 2, common.State_SERVING)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			KVMs, err := listKVMs(c, req)
			So(err, ShouldBeNil)
			So(KVMs, ShouldResemble, []*crimson.KVM{
				{
					Name:       "kvm 1",
					Vlan:       1,
					Platform:   "platform 1",
					Rack:       "rack 1",
					Datacenter: "datacenter 1",
					MacAddress: "00:00:00:00:00:01",
					Ipv4:       "0.0.0.1",
					State:      common.State_FREE,
				},
				{
					Name:       "kvm 2",
					Vlan:       2,
					Platform:   "platform 2",
					Rack:       "rack 2",
					Datacenter: "datacenter 2",
					MacAddress: "00:00:00:00:00:02",
					Ipv4:       "0.0.0.2",
					State:      common.State_SERVING,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
