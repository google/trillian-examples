// Copyright 2020 Google LLC. All Rights Reserved.
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

package rom

import (
	"fmt"
	"time"

	"github.com/perlin-network/life/exec"
	wasm_validation "github.com/perlin-network/life/wasm-validation"
)

// Most of the code below was copied from https://github.com/perlin-network/life/blob/master/main.go

// Resolver defines imports for WebAssembly modules ran in Life.
type Resolver struct{}

// ResolveFunc defines a set of import functions that may be called within a WebAssembly module.
func (r *Resolver) ResolveFunc(module, field string) exec.FunctionImport {
	switch module {
	case "env":
		switch field {
		case "__life_ping":
			return func(vm *exec.VirtualMachine) int64 {
				return vm.GetCurrentFrame().Locals[0] + 1
			}
		case "__life_log":
			return func(vm *exec.VirtualMachine) int64 {
				ptr := int(uint32(vm.GetCurrentFrame().Locals[0]))
				msgLen := int(uint32(vm.GetCurrentFrame().Locals[1]))
				msg := vm.Memory[ptr : ptr+msgLen]
				fmt.Printf("[app] %s\n", string(msg))
				return 0
			}
		case "print_i64":
			return func(vm *exec.VirtualMachine) int64 {
				fmt.Printf("[app] print_i64: %d\n", vm.GetCurrentFrame().Locals[0])
				return 0
			}
		case "print":
			return func(vm *exec.VirtualMachine) int64 {
				ptr := int(uint32(vm.GetCurrentFrame().Locals[0]))
				len := 0
				for vm.Memory[ptr+len] != 0 {
					len++
				}
				msg := vm.Memory[ptr : ptr+len]
				fmt.Printf("[app] print: %s\n", string(msg))
				return 0
			}

		default:
			panic(fmt.Errorf("unknown field: %s", field))
		}
	default:
		panic(fmt.Errorf("unknown module: %s", module))
	}
}

// ResolveGlobal defines a set of global variables for use within a WebAssembly module.
func (r *Resolver) ResolveGlobal(module, field string) int64 {
	switch module {
	case "env":
		switch field {
		case "__life_magic":
			return 424
		default:
			panic(fmt.Errorf("unknown field: %s", field))
		}
	default:
		panic(fmt.Errorf("unknown module: %s", module))
	}
}

// bootWasm prepares the VM with the provided wasm binary by calling the function
// named by entryPoint.
//
// Note that if the VM is unable to find the specified entrypoint (or none is
// specified), the the VM will fall back to attempting to execute the first
// function it finds in the input binary.
//
// Visit the [Life](https://github.com/perlin-network/life) repo for more info.
func bootWasm(entryPoint string, input []byte) error {

	if err := wasm_validation.ValidateWasm(input); err != nil {
		return err
	}

	// Instantiate a new WebAssembly VM with a few resolved imports.
	vm, err := exec.NewVirtualMachine(input, exec.VMConfig{
		DefaultMemoryPages:   128,
		DefaultTableSize:     65536,
		DisableFloatingPoint: false,
	}, new(Resolver), nil)
	if err != nil {
		return err
	}

	// Get the function ID of the entry point function to be executed.
	entryID, ok := vm.GetFunctionExport(entryPoint)
	if !ok {
		fmt.Printf("Entry function %s not found; starting from 0.\n", entryPoint)
		entryID = 0
	}

	start := time.Now()

	// If any function prior to the entry function was declared to be
	// called by the module, run it first.
	if vm.Module.Base.Start != nil {
		startID := int(vm.Module.Base.Start.Index)
		_, err := vm.Run(startID)
		if err != nil {
			vm.PrintStackTrace()
			return err
		}
	}
	// Run the WebAssembly module's entry function.
	ret, err := vm.Run(entryID)
	if err != nil {
		vm.PrintStackTrace()
		return err
	}
	end := time.Now()

	fmt.Printf("return value = %d, duration = %v\n", ret, end.Sub(start))
	return nil
}
