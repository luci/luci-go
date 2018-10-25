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
	"context"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/luci/mp/api/suppliers/v1"
)

// NewSuppliers returns a new suppliers RPC server.
func NewSuppliers() suppliers.SuppliersServer {
	return &suppliers.DecoratedSuppliers{
		Service: &Suppliers{},
	}
}

// Suppliers is an RPC server for manipulating suppliers.
// Implements suppliers.SuppliersServer.
type Suppliers struct {
}

// Create creates a new supplier.
// Implements suppliers.SuppliersServer.
func (*Suppliers) Create(c context.Context, req *suppliers.CreateSupplierRequest) (*suppliers.Supplier, error) {
	return &suppliers.Supplier{}, nil
}

// Delete deletes an existing supplier.
// Implements suppliers.SuppliersServer.
func (*Suppliers) Delete(c context.Context, req *suppliers.DeleteSupplierRequest) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}

// Get gets an existing supplier.
// Implements suppliers.SuppliersServer.
func (*Suppliers) Get(c context.Context, req *suppliers.GetSupplierRequest) (*suppliers.Supplier, error) {
	return &suppliers.Supplier{}, nil
}
