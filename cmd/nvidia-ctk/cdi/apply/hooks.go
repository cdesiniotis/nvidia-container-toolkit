/**
# SPDX-FileCopyrightText: Copyright (c) NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package apply

import (
	"context"
	"fmt"
	"io"
	"strings"

	"tags.cncf.io/container-device-interface/pkg/cdi"
	"tags.cncf.io/container-device-interface/specs-go"

	"github.com/NVIDIA/nvidia-container-toolkit/internal/edits"
)

// applyHookMode converts the hooks associated with the requested devices into
// a list of shell commands and writes them, one per line, to the output. The
// resulting commands can be executed as a container prestart hook by a
// non-OCI-compliant runtime. Each command maps one-to-one to a hook from the
// device's CDI spec.
func (m command) applyHookMode(_ context.Context, opts *options, registry *cdi.Cache, outputWriter io.Writer) error {
	hooks, err := m.getHooks(opts, registry)
	if err != nil {
		return err
	}

	for _, hook := range hooks {
		fmt.Fprintln(outputWriter, hookToCommand(hook, opts.containerRoot))
	}

	return nil
}

// getHooks returns the ordered list of hooks for the requested devices,
// combining the spec-level and device-level container edits in the same way
// that they would be applied to an OCI spec.
func (m command) getHooks(opts *options, registry *cdi.Cache) ([]*specs.Hook, error) {
	allEdits := &ContainerEdits{
		ContainerEdits: edits.NewFactory().New(),
	}
	seenSpecs := map[*cdi.Spec]struct{}{}

	var unresolved []string
	for _, device := range opts.devices {
		d := registry.GetDevice(device)
		if d == nil {
			unresolved = append(unresolved, device)
			continue
		}

		if _, ok := seenSpecs[d.GetSpec()]; !ok {
			seenSpecs[d.GetSpec()] = struct{}{}
			allEdits.Append(&cdi.ContainerEdits{
				ContainerEdits: &d.GetSpec().ContainerEdits,
			})
		}
		allEdits.Append(&cdi.ContainerEdits{
			ContainerEdits: &d.ContainerEdits,
		})
	}

	if len(unresolved) > 0 {
		return nil, fmt.Errorf("unknown devices requested: %v", unresolved)
	}

	return allEdits.Hooks, nil
}

// hookToCommand converts a single CDI hook into an executable shell command.
func hookToCommand(hook *specs.Hook, containerRoot string) string {
	var parts []string

	// Any environment variables associated with the hook are emitted as leading
	// assignments so that the command reproduces the hook invocation.
	for _, env := range hook.Env {
		if key, value, found := strings.Cut(env, "="); found {
			parts = append(parts, key+"="+value)
		} else {
			parts = append(parts, env)
		}
	}

	parts = append(parts, hook.Path)

	// Args[0] is the argv[0] placeholder (the program name); the actual
	// executable is given by Path, so it is dropped here.
	if len(hook.Args) > 1 {
		for _, arg := range hook.Args[1:] {
			parts = append(parts, arg)
		}
	}

	// If containerRoot is non-empty, a --container-root flag is appended to
	// the command so that the hook operates on the specified container root.
	if containerRoot != "" {
		parts = append(parts, "--container-root", containerRoot)
	}

	return strings.Join(parts, " ")
}
