// Copyright 2015 James Cote and Liberty Fund, Inc.
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package credentials provides oauth2 credentials for
// batch requests.
package credentials

import (
	"golang.org/x/oauth2"
)

// Oauth2Credentials wraps an oauth2.TokenSource
type Oauth2Credentials struct {
	oauth2.TokenSource
}

// Authorization returns value needed for an Authorization header
func (o *Oauth2Credentials) Authorization() (string, error) {
	tk, err := o.Token()
	if err != nil {
		return "", err
	}
	return tk.Type() + " " + tk.AccessToken, nil
}
