// Runs inside a Data App's sandboxed frame, right after the Anfra SDK's IIFE bundle and before any
// author code. It provisions the SDK from the data the demo server inlined, with a Backend that
// forwards every call to the Shell over postMessage, then installs it as the `Anfra` global.
(function bootstrap () {
  var provision = JSON.parse(document.getElementById('anfra-provision').textContent);
  var sdk = window.AnfraSdk;
  var pending = new Map();
  var nextId = 1;

  function abortError () {
    var err = new Error('Aborted');
    err.name = 'AbortError';
    return err;
  }

  // Errors cross the bridge as { name, message }; rebuild the SDK's class so an author can tell a
  // permission failure from a broken query.
  function toError (error) {
    var Kind = { QueryError: sdk.QueryError, PermissionError: sdk.PermissionError, TransportError: sdk.TransportError }[error && error.name];
    return Kind ? new Kind(error.message) : new sdk.QueryError((error && error.message) || 'The query failed.');
  }

  window.addEventListener('message', function (event) {
    if (event.source !== window.parent) return;
    var message = event.data;
    if (!message || message.type !== 'anfra:response') return;
    var call = pending.get(message.id);
    if (!call) return;
    pending.delete(message.id);
    if (message.ok) call.resolve(message.result);
    else call.reject(toError(message.error));
  });

  function call (method, request, signal) {
    return new Promise(function (resolve, reject) {
      if (signal.aborted) {
        reject(abortError());
        return;
      }
      var id = nextId++;
      pending.set(id, { resolve: resolve, reject: reject });
      signal.addEventListener('abort', function () {
        if (!pending.delete(id)) return;
        window.parent.postMessage({ type: 'anfra:cancel', id: id }, provision.shellOrigin);
        reject(abortError());
      }, { once: true });
      window.parent.postMessage({
        type: 'anfra:request', id: id, method: method, request: request,
      }, provision.shellOrigin);
    });
  }

  var backend = {
    submitQuery: function (request, signal) {
      return call('submitQuery', request, signal).then(function (result) {
        // Dates don't survive JSON; provenance's `executedAt` is one.
        if (result.debug && result.debug.executedAt) {
          result.debug.executedAt = new Date(result.debug.executedAt);
        }
        return result;
      });
    },
    fieldSuggestions: function (request, signal) {
      return call('fieldSuggestions', request, signal);
    },
  };

  var instance = sdk.createSdk({
    datasets: provision.datasets,
    user: provision.user,
    backend: backend,
  });

  // Inspection: while the Shell's Inspect panel is open, post a snapshot of every app on each
  // change. The instance is the only way to reach an app the author built, so wrap createApp to
  // subscribe to each one as it appears.
  var watched = false;
  var timer = null;

  function postSnapshot () {
    timer = null;
    if (!watched) return;
    try {
      window.parent.postMessage({
        type: 'anfra:inspect',
        apps: instance.apps.map(function (app) { return app.toInspectJSON(); }),
      }, provision.shellOrigin);
    } catch (err) {
      // A snapshot that will not clone must not break the author's app.
    }
  }

  function scheduleSnapshot () {
    if (watched && timer === null) timer = setTimeout(postSnapshot, 50);
  }

  var createApp = instance.createApp.bind(instance);
  instance.createApp = function (declaration) {
    var app = createApp(declaration);
    app.subscribe(scheduleSnapshot);
    scheduleSnapshot();
    return app;
  };

  window.addEventListener('message', function (event) {
    if (event.source !== window.parent) return;
    var message = event.data;
    if (!message || message.type !== 'anfra:inspect-watch') return;
    watched = !!message.open;
    if (watched) postSnapshot();
  });

  sdk.installSandbox(instance, window);
  window.parent.postMessage({ type: 'anfra:ready' }, provision.shellOrigin);
}());
