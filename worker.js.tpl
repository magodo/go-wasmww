importScripts(location.origin + '/wasm_exec.js');

let initialMsg = true;
let waInit; 

onmessage = async (e) => {
    if (initialMsg) {
        initialMsg = false;
        const data = e.data;
        const argv = data.argv;
        const env = data.env;
        const path = data.path;
        const go = new Go();
        go.argv = argv;
        go.env = env;
        waInit = WebAssembly.instantiateStreaming(fetch(location.origin+"/"+path), go.importObject).then((result) => {
            go.run(result.instance);
        });
        return
    }

    await waInit;
    {{.CbName}}(e);
}
