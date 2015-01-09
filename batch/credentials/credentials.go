// Copyright 2015 James Cote and Liberty Fund, Inc.
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package credentials

import (
	"net/http"

	"golang.org/x/oauth2"
)

type Oauth2Credentials struct {
	oauth2.TokenSource
}

func (o *Oauth2Credentials) Authorization() (string, error) {
	tk, err := o.Token()
	if err != nil {
		return "", err
	}
	return tk.Type() + " " + tk.AccessToken, nil
}

func (o *Oauth2Credentials) SetAuthHeader(r *http.Request) error {
	tk, err := o.Token()
	if err != nil {
		return err
	}
	tk.SetAuthHeader(r)
	return nil
}
