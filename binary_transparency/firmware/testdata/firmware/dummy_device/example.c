// This is an example firmware for the dummy device.
//
// The dummy device expects a WASM firmware payload, so this code must be
// compiled to WASM before use.
// You can use https://wasdk.github.io/WasmFiddle/ to play around with this
// code, compile, and download the compiled .wasm file.
//
// The dummy device expects to find a non-mangled entry point called
// main(), and there is no support for stdlib functions, just a single
// print function with the signature shown below.

// print is exported by the WASM vm.
void print(const char*);

int main() {
  print("Hello world!");
  return 0;
}