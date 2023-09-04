importScripts(location.origin + '/wasm_exec.js');

const go = new Go();
go.argv = {{.ArgsToJS}}
go.env = {{.EnvToJS}}
WebAssembly.instantiateStreaming(fetch(location.origin + "/{{.Path}}"), go.importObject).then((result) => {
    go.run(result.instance);
});
