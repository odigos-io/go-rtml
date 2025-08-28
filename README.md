# go-rtml

golang real time memory limiter to guard against OOM in go code.

This package is all about the STABILITY of your go application under memory pressure.

## Motivation

Your golang applications need memory (RAM) to run. In an utopic world, you would have infinite memory and never need to think or worry about how much memory resources are consumed. In the real world, memory is a limited resource which has to be managed carefully.

When your process is under memory pressure for any reason, you - the application developer, is responsible to react be applying back-pressure and avoid processing new work items which can increase the memory pressure and lead to OutOfMemory brutal termination of the process.

## How it works

- Call `rtml.IsMemLimitReached()` in the entry points to your application
- Do it where you have the ability to reject, drop, or apply back-pressure to your senders.
- Prefer to call it as soon as possible, before any expensive allocations are made.

This simple function will tell you if the memory limit is reached and allow you to react, for example: 
- apply back-pressure to the sender by rejecting the work item. it is expected to be retried after some time (where hopefully, memory pressure has already reduced to normal levels).
- drop the work item if the system cannot handle it under the current memory pressure.
- notify some monitoring system about this event for further investigation or automatic scaling.

## Usage

```go
package main

import (
	rtml "github.com/odigos-io/go-rtml"
)

func requestHandler() {
    if rtml.IsMemLimitReached() {
        return errResourceExhausted, "Memory limit reached"
    }

    // process request, which might be be allocation heavy.
    return nil // success, no memory limit to back-pressure.
}
```

and build your application with the following ldflag: "-checklinkname=0".

## Testing

This project includes a comprehensive test framework that runs tests in isolated containers to verify memory limit behavior. The test framework is located in the `testframework/` directory as a separate Go module.

### Quick Test Run

```bash
# Navigate to test framework directory
cd testframework

# Run the complete test suite
./scripts/run-tests.sh

# Or use make
make docker-run-tests
```

### Test Types

- **Memory Allocation Tests**: Verify basic memory allocation within limits
- **Memory Limit Tests**: Ensure memory limits are properly enforced
- **Stress Tests**: Test memory management under sustained load

For detailed documentation, see [testframework/README.md](testframework/README.md).

## About ldflags="-checklinkname=0"

This package uses `go:linkname` to access the internal state of the go runtime.

This is considered bad practice and not recommended by the go team, thus the ldflag to warn you.

Having said that, it does address a hard to solve real-world problem, in a way that satisfies tight performance requirements and acurate reaction to memory pressure.

Be aware that this practice, while working, can break unexpectedly by internal changes to the go runtime implementation without notice. Test your application with every new go version, weight the benefits, risks and alternatives, and evaluate if this risk is acceptable for you.

## What is Memory Limit?

Memory limit is a feature of the operating system, that will terminate your go process (or group of processes) if they are collectivly consuming more memory than the allowed limit.

It is very common in kuberenetes to set resource request and limit on memory and cpu. Under the hood it uses cgroups which is a linux kernel feature to achieve this goal.

## Why Set Memory Limit?

If you are a developer, setting memory limit is a chore that you probably don't want to participate in. But if you are a devops engineer, you care about the overall stability of the system, and probably learned the easy or hard way that not setting any memory limit is a big no-no and recipe for disaster.

If the application suddenly starts consuming a lot of memory and there are no limits in place, this memory will have to come from somewhere. it will start eating into the operating system's free memory, which can end up in degraded performance, resource exhaustion, and harm other applications running on the same machine.

## Why it's Important to Avoid Crashing due to OOM

Giving that you (or your opeartion engineer) have set memory limit (best practice) - you are guarded against the application causing general system or machine instability which is a good start.
But it creates a new problem - what if the application itself reaches the memory limit and get terminated? How do we guarentee the application stability in this case?

Why it is so important to not crash the application due to OOM?

- Some platforms (like kubernetes) handles automatic autoscaling which can address issues of memory exhaustion, but for it to trigger, the container needs to be up and report metrics. When a container crashes, the relevant metrics might not be recorded in time and prevent the autoscaler from starting more replicas or increasing the resources which ends up in a death spiral.
- In mature production environments, crashing pods will show up as alerts or in dashboards, creating operational noise and alert fatigue.
- Crashing with OOM can cause data loss or corruption since the process is terminated abruptly and does not have a chance to execute a graceful shutdown.
- If the application crashes too often, it can cause service downtime and fail to serve requests in general.
- Degraded user experience, confidence, and trust over time.

For these reasons, we aim to never crash the application due to OOM. Easier said than done :/

## Memory Managment in Go

### CGroup Memory Limit

CGroup is a linux kernel feature that allows to limit the resources (memory, cpu, etc) that a group of processes can consume. When you run a container, usually all the processes inside this container are part of the same cgroup and all share the same memory limit.

A memory limit for the cgroup (container) is a number you can or cannot control, which is a hard limit - once crossed, processes in the container will be terminated and stabilty will be degraded.

Go applications are commonly run in containers, and as a single process, thus we will assume for the rest of this document that the memory limit is set on the container level to some predefined value and there is a single go process running in the container.

Our goal (in terms of the CGroup memory limit) is to never let the go process consume more operating system memory than the container memory limit.

### Operating System Memeory

Under the hood, go is just a normal process running on the operating system. When it needs memory (for the heap, stack, large objects, etc), or want to release some memory acuired before, it calls the operating system api (via syscalls) to "map" this memory, which is considered an expensive call.

The operating system itself only manages memory in chunks called "pages" (typically 4KB in size). Go will try to minimize the number of these calls, and manage memory from the operating system (both allocations and releases) in large chunks so they are ready for future use.

For the memory accounting to towards the cgroup memory limit, only "resident set" memory is considered. Resident set memory is the memory that is currently being used by the process. When a page is mapped, it does not count in the resident set and thus does not contribute to the memory accounting. only once the application first "writes" to this page, it will be counted as resident set memory and bring the process closer to the limit (at least in linux).

This memory is not automatically released from the resident set and impact how much we are close to the limit. go runtime needs to explicitly notify the operating system that the memory is now unused (costly operation).

### Appliction Runtime Memory

In your go code, you usually allocate memory by using `make`, creating new objects, manipulating strings and slices, etc.

These are very frequent, thus it is highly optimized to be fast and cheap, avoiding "expensive" work on the hot path. Go runtime will pre-allocate a "span" of memory which is ready to host a bulk of objects. most allocations are made from this "span" real quick, and when the span is full, a new one is aquired.

### Garbage Collection

Go garbage collector will take care of calling the 

- real-time memory accounting
- configuration from user (`GOGC` and `GOMEMLIMIT`)
- 
