// Copyright 2017 Istio Authors
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

package pilot

import (
	"fmt"
	"testing"
)

func TestHttp(t *testing.T) {
	// Auth is enabled for d:80, and disabled for d:8080 using per-service policy.
	// We expect request from non-envoy client ("t") to d:80 should always fail,
	// while to d:8080 should always success.
	srcPods := []string{"a", "b", "t"}
	dstPods := []string{"a", "b", "d"}
	ports := []string{"", "80", "8080"}
	if !tc.Kube.AuthEnabled {
		// t is not behind proxy, so it cannot talk in Istio auth.
		dstPods = append(dstPods, "t")
		// mTLS is not supported for headless services
		dstPods = append(dstPods, "headless")
	}

	logs := newAccessLogs()

	// Run all request tests.
	t.Run("request", func(t *testing.T) {
		for _, src := range srcPods {
			for _, dst := range dstPods {
				if src == "t" && dst == "t" {
					// this is flaky in minikube
					continue
				}
				for _, port := range ports {
					for _, domain := range []string{"", "." + tc.Kube.Namespace} {
						testName := fmt.Sprintf("%s->%s%s_%s", src, dst, domain, port)
						runRetriableTest(t, testName, defaultRetryBudget, func() error {
							reqURL := fmt.Sprintf("http://%s%s:%s/%s", dst, domain, port, src)
							resp := ClientRequest(src, reqURL, 1, "")
							// Auth is enabled for d:80 and disable for d:8080 using per-service
							// policy.
							if src == "t" &&
								((tc.Kube.AuthEnabled && !(dst == "d" && port == "8080")) ||
									dst == "d" && (port == "80" || port == "")) {
								if len(resp.ID) == 0 {
									// Expected no match for:
									//   t->a (or b) when auth is on
									//   t->d:80 (all the time)
									// t->d:8000 should always be fine.
									return nil
								}
								return errAgain
							}
							logEntry := fmt.Sprintf("HTTP request from %s to %s%s:%s", src, dst, domain, port)
							if len(resp.ID) > 0 {
								id := resp.ID[0]
								if src != "t" {
									logs.add(src, id, logEntry)
								}
								if dst != "t" {
									if dst == "headless" { // headless points to b
										if src != "b" {
											logs.add("b", id, logEntry)
										}
									} else {
										logs.add(dst, id, logEntry)
									}
								}
								return nil
							}
							if src == "t" && dst == "t" {
								// Expected no match for t->t
								return nil
							}
							return errAgain
						})
					}
				}
			}
		}
	})

	// After all requests complete, run the check logs tests.
	if len(logs.logs) > 0 {
		t.Run("check", func(t *testing.T) {
			logs.checkLogs(t)
		})
	}
}
