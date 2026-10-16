# Method naming

How we name methods, and why `executor.Submit(run)` and `w.Write(p)` are both right.

## A method is a message

A call is a message sent to the receiver, as in Smalltalk's view of objects; the method is named
as the message. The verb supplies the roles; the receiver and the arguments take them:

- **"submit"** has someone submitting, something submitted, and someone it is submitted to.
  In `executor.Submit(run)`, the run is what is submitted and the executor is who it is
  submitted to.
- **"write"** has a writer and what is written. In `w.Write(p)`, the writer writes `p`.

Which role the receiver takes is not fixed by Go's syntax: it is read from what the verb and the
receiver mean together. An executor can only be submitted to; a writer is what writes. So the
receiver may be the actor (`w.Write(p)`, `client.Send(msg)`, `h.ServeHTTP(w, r)`) or what is acted
on (`pool.Submit(task)`, `stack.Push(x)`, `sem.Acquire(ctx, n)`). Both are idiomatic.

**The test is the call site**: read `receiver.Method(args)` aloud. If it says what happens —
"submit the run to the executor" — the name is right.

## Hooks are not methods

A hook is a function field a host fills in, which the framework calls: `Mounting.Admit`,
`Mounting.Authorize`, `Mounting.Bind`. There is no receiver to read the roles from, and the reader
is the host writing it, so **a hook is named for what the host does when called**: the host admits
the caller, authorizes the call, binds the caller to an invocation.

A hook named for what the *framework* does reads backwards: `Mounting.Submit` would be the host
being submitted to, among fields that are each the host acting.

## Name the operation, not the result

`executor.Submit(run)` returns an execution, but is not `CreateExecution`: it names what is done
(work handed over, which may be refused or held) rather than the object answered. A name taken
from the result hides what the operation may do on the way.

## One action, one name, at every layer

When layers wrap each other, the same action keeps its name, the receiver saying who acts —
`io.Reader.Read` on a file, a buffer or a gzip reader; `http.Handler.ServeHTTP` on each
middleware, calling the next's. A run is executed by `Page` or `Stream`: on the op's `Run`, and on
the executor's `Execution`, which wraps it. Inventing a name per layer (`read`, `deliver`,
`perform`) hides that it is one action, and each new name invites the question of what it means.

## Go's own conventions

Beside these, what Go's style guides say (Effective Go, Code Review Comments):

- no `Get` prefix on a getter: `Owner()`, not `GetOwner()`;
- a one-method interface is the method plus "-er": `Reader`, `Writer`;
- MixedCaps; short names; no repeating the package's name (`jobs.Service`, not
  `jobs.JobsService`).

And from command–query separation: a command — it changes state — is a verb (`Submit`,
`Write`); a query — it answers state — a noun or an adjective (`Len`, `Owner`, `IsEmpty`).
