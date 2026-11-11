// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"sync"

	"github.com/mendixlabs/mxcli/sdk/versions"
)

var resolveOnce sync.Once

// resolveVersions fills each topic's MinVersion and Requires from the version
// registry, once, before any lookup returns a topic. A feature the registry
// does not have is left out here and reported by TestSyntaxFeaturesResolve.
func resolveVersions() {
	resolveOnce.Do(func() {
		reg, err := versions.Load()
		if err != nil {
			return
		}
		for i := range registry {
			f := &registry[i]
			var primary *versions.SemVer
			for _, key := range f.Features {
				area, name, ok := cutFeatureKey(key)
				if !ok {
					continue
				}
				e, found := reg.Lookup(area, name)
				if !found {
					continue
				}
				r := Requirement{Feature: key, Since: e.MinVersion.String(), MDL: e.MDL}
				if e.MaxVersion != nil {
					r.Until = e.MaxVersion.String()
				}
				f.Requires = append(f.Requires, r)
				if primary == nil {
					v := e.MinVersion
					primary = &v
				}
			}
			if primary != nil {
				f.MinVersion = primary.String()
			}
		}
	})
}

func cutFeatureKey(key string) (area, name string, ok bool) {
	for i := 0; i < len(key); i++ {
		if key[i] == '/' {
			return key[:i], key[i+1:], i > 0 && i < len(key)-1
		}
	}
	return "", "", false
}
