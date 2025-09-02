# go-rtml

- **Shortened** - Go Real Time Memory Limiter.
- **Goal** - Memory STABILITY - avoid OutOfMemory (OOM) brutal termination of your go application.
- **Mechanism** - a function you can call in your code, before handling any new work items. based on a boolean response, you should either accept or reject before making expensive allocations.
- **Performant** - cheap to call so you can check in real time and per incoming request instead of using inacurate heuristic sampling and expensive "stop the world" (ReadMemStats) alternatives.
- **Accurate** - use the exact same values as the go runtime, to align perfectly with it's algorithm and state.

## Motivation

Your golang applications need memory (RAM) to run. In an utopic world, you would have infinite memory and never need to think or worry about how much memory resources are consumed. In the real world, memory is a limited resource which has to be managed carefully.

When your process is under memory pressure for any reason, you - the application developer, is responsible to avoid processing new work items which can increase the memory more and lead to OutOfMemory brutal termination of the process. 

How to do that in a safe and performant way you ask? introducing 🎉 `go-rtml` 🎉

At [Odigos](https://odigos.io), we deploy a pipeline for collecting and processing high volumes of logs, metrics and traces, using OpenTelemetry Collectors. Our stability journey has been long and challenging, leading us deep into some interesting rabbit holes in the go runtime. This module is our state-of-the-art solution to avoid OutOfMemory which is the result of long research and has proven results in production systems.

## How it works

- Make sure you set `GOMEMLIMIT` environment variable in alignment to your container memory limit.
- Call `rtml.IsMemLimitReached()` in the entry points to your application, (or on checkpoint before doing some potentially expensive allocations).
- Do it where you have the ability to reject, drop, or apply back-pressure to your senders.
- Prefer calling it as soon as possible, before any expensive allocations are made.

This simple function will give you just one boolean result. `false` means memory is below the limit and the work can be accepted, `true` means memory is above the limit and processing new work is a risk for Out Of Memory, thus needs to be rejected or dropped.

- If you want some internal insights into the numbers that makes up this final boolean decision, call `GetMemLimitRelatedStats` and follow the below documentation.

## Usage

```go
package main

import (
	rtml "github.com/odigos-io/go-rtml"
)

func requestHandler() error {
    if rtml.IsMemLimitReached() {
        return errResourceExhausted, "Memory limit reached"
    }

    // process request, which might be be allocation heavy.

    return nil // success, no memory limit to back-pressure.
}
```

and build your application with the following ldflag: `"-checklinkname=0"`.

## About `ldflags="-checklinkname=0"`

This package uses `go:linkname` to access the internal state of the go runtime.

This is [considered bad practice and not recommended by the go team](https://github.com/golang/go/issues/67401), thus the ldflag to warn you.

Having said that, it does address a hard to solve real-world problem, where alternatives cannot guarantee the same level of accuracy and performance.

Be aware that there is not forward or backward compatibility guarantee. Test your application with every new go version, weight the benefits, risks and alternatives, and evaluate if this risk is acceptable for you.

We run daily tests for all version of go above 1.23 to ensure that the package is compatible and stable.

## Call Frequency

Calling `rtml.IsMemLimitReached()` is considered "cheap", since it is doing the same "work" that go runtime is doing anyway once every few KBs of heap allocations.

Under the hood, this function will access few atomic variables, and make a simple computation, so it's not free.

Calling it one every incoming request is ok, but keep in mind that it's not free, so try to keep it to a minimum and batch if possible. Test your application under load to exaimne it's performance.

## Alternatives

Popular alternatives are:

- Sampling the result of `runtime.ReadMemStats()` in a regular interval. This is problematic since any allocations done during 2 interval samples might be enough to cause Out Of Memory before there is a chance to react. This function will stop all your go routines while doing some non-trivial computations work, and calling it too frequently can impact the performance of your application, while not calling it frequently enough can harm stability under high load.

- Using kernel memory values for the process, for example with [gopsutil](https://github.com/shirou/gopsutil). The values are read from files in `/proc/...` which requires file open and read thus not sutabile fo call on each request. These numbers only reflects OS point of view which may be incomplete without the internal go runtime state.

## What is Kernel Memory Limit?

Kernel Memory Limit is a feature of the operating system, that allows you to set a hard limit in number of bytes for the amount of phsyical memory (RAM) that a group of processes (like all those that run together in a container) can consume collectively.

Once reached, the operating system will terminate processes with "Out Of Memory (OOM)" error.

In kubernetes, it is applied on the pod manifest under container resoure limits:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
spec:
  containers:
    - name: my-container
      resources:
        limits:
          memory: 512Mi
```

## Why Set Memory Limit?

Without memory limits, your go application can consume too much memory and eventually eat into the operating system's limited memory pool, shared by all containers and the operating system itself. If not handled correctly, this can lead to degraded performance of other unrelated appliications or containers, or even the host itself.

While never planed, memory usage spikes can occur due to bugs, memory leaks, environment changes over time(configurations, versions, data, etc.), un-optimal algorithms and implementations, exceptional high load from external sources, downstream services back-pressure, and many other reasons.

Since we can never guarantee that go application will consume decent amount of memory, and system stability is a top priority over any one specific service - it is common and recommended to have memory limits in place, especially in production environments where downtime is a major concern.

## Why it's Important to Avoid Crashing due to OOM

- Some platforms (like kubernetes) handles resource exhaustion automatically, by scaling up the number of replicas. For it to trigger, the application needs to be alive during the metrics collection time, and report the high memory usage. If the container crashes before it had a chance to report, new replicas will not start, causing a death spiral.
- In mature production environments, crashing pods will show up as alerts or in dashboards, creating operational noise and alert fatigue.
- Crashing with OOM can cause data loss or corruption since the process is terminated abruptly and does not have a chance to execute a graceful shutdown and flush it's memory queues.
- If the application crashes too often, it can cause SLA violations and even degrade to complete downtime. A service that crashes once in a while, can escalate to a major outage quickly, for example if the load is increased in a burst.
- Degraded user experience, confidence, and trust over time.

For these reasons, we aim to never crash the application due to OOM. Easier said than done.

## How Check the Limit in Real-Time Works?

### Container (Kernel) Level Memory Limit

Container level memory limit are usually enforced in the kernel (operating system), on a cgroup (or in containerized environments - for all processes in the container).

For simplicity, and since go usually runs as a single process (with multiple OS threads), it's sometimes enought to attribute the container limit to the process.

### GOMEMLIMIT

GOMEMLIMIT is a way to reflect the container memory limit to the go runtime and allow it to call the garbage collector at the right times to avoid crossing this limit. Unlike the Kernel Memory Limit, GOMEMLIMIT is a soft limit - meaning the runtime is designed in such a way that memory usage can exceed this limit under normal conditions.

You can, for example, set GOMEMLIMIT to 80% of the container memory limit, which for 512MB container limit, will leave you with 410MB for normal usage, and ~100MB for spikes and safety margins.

This number is somehow arbitrary and encapsulates a trade-off between stability and costs (memory usage). It's not a magic number, and you can tune it to your needs.

### Resident Set Memory and "Ready" Memory

The memory that is counted towards the container (cgroup) memory limit is called "resident set" memory, which are kernel memory pages backed by physical memory. This is the "Important" value that the kernel will match against the limit to trigger OutOfMemory terminations.

Go runtime on it's side, tracks the number of pages it considers as "Ready". A memory page is ready if the runtime can use it to make allocations. A ready page is usually backed by physical memory and contributes to the resident set memory, but not always (the operating system is quite efficient in delaying the physical memory allocation until it is really needed).

Therefore: `ResidentSet` (kernel) <= `MappedReady` (go runtime)

```
if MappedReady < GOMEMLIMIT {
    return false // memory limit not reached
}
```

The total resident set is less then mapped ready, meaning that if go runtime hasn't mapped this amount of memory, it's still below the limit and eveything is fine. This check is expected to quickly terminate the check in most cases under normal conditions when memory is not under pressure.

### HeapFree

When go runtime needs to allocate new kernel memory for it's heap, or when memory is no longer used and can be freed, it calls the operating system api (syscalls) to do so which is considered expensive.

To avoid calling syscalls too often, go runtime will "recycle" big chunks of empty memory after garbage collection, keeping it in "ready" state for future allocations, while it is still counted towards the kernel "resident set" hard memory limit.

The amount of such memory is called "HeapFree" in the go runtime. It is increased after garbage collection (during the sweep phase) and decreased when new allocations are made or when unused memory is "returned" back to the operating system after some time (scavenged in go runtime terms).

Since it can safely accumadate this amount of future allocations which are already "charged" for, we deduct it from the mapped ready count and compare this to the GOMEMLIMIT.

```
if (MappedReady - HeapFree) < GOMEMLIMIT {
    return false // memory limit not reached
}
```

It can quickly catch issues where memory usage was high (MappedReady > GOMEMLIMIT) but after garbage collection, there is now "HeapFree" memory to balance it.

### HeapGoal

The final check is expected to trigger very rarely, only when the memory usage is being constantly kept above or near the GOMEMLIMIT. This is an unhealthy scenario that this module is designed to warn about.

In "normal" operations, the memory usages grows more and more - and garbage collection is triggered before or near the GOMEMLIMIT. After garbage collection, there is now a lot of free memory, to use for future allocations.

In the "bad scenario", your program is still keeping references to a lot of memory (for example - in queues, memory caches, etc) and after running garbage collection, the consumed memory is still above the GOMEMLIMIT.

Quoting from the go runtime source code comments:

> There's honestly not much we can do here but just trigger GCs continuously
> and let the CPU limiter reign that in. Something has to give at this point.

This is where it's most critical to reject any new allocations since we are already above the GOMEMLIMIT value meaning we are getting close to kernel memory limit and OutOfMemory brutaltermination.

The way we check it is by doing exactly what go runtime is doing - calculate the heap goal and compare it with heap live.

```
if HeapLive < HeapGoal {
    return false // memory limit not reached
}
```

We use the heap goal as a proxy to the above "bad scenario" - heap live should be maintained below the goal by the garbage collector in "healthy" operation, and failing to do so indicates that garbage collection is not effective in reducing the memory usage - thus the pressure.
