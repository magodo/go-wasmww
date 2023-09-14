addEventListener("connect", (e) => {
    const port = e.ports[0];
    self.recent_port = port;
    port.start();
});

importScripts(location.origin + '/wasm_exec.js');

const go = new Go();
go.argv = {{.ArgsToJS}}
go.env = {{.EnvToJS}}
WebAssembly.instantiateStreaming(fetch("{{.Path}}"), go.importObject).then((result) => {
    go.run(result.instance);
});
