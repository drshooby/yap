# Devlog

This is where my developer yapping in `yap` exists. Hopefully, this serves to be informative for not only my future self but also anybody that stumbles across this project.

I'm taking my time with this project since I do a lot of agentic coding day-to-day and this project is where I plan to practice my _artisanal_ coding skills (and be more comfortable writing).

The rule I've set up with Claude is it pushes back when I try to ask it to write business logic code for me (tests are fine as are tedious struct definitions) and highlights what Go concepts I'm learning as we move along.

---

# Project init

#### Sept 12/13

I had my initial `README.md` brainstorm with Claude that I needed turned more into a more "actionable" plan for what this project was to be. We set up the initial scaffolding over these days and issues were created so I can try to tackle an issue during a free evening. Lastly we added the basic event schema.

# JSONL writer

#### Sept 14

It took me way too long to understand Go's `io.Writer` usage. Claude tried leading me astray as a "learning opportunity" by having me start with the bare writer when `bufio` handled buffers for me.

I also spent a solid 30 minutes bouncing between Claude and Google for help on the difference between marshalling and encoding when writing. In the end it didn't matter... I settled for encoding.

What did I learn today? First would be how flushing works with `bufio.Writer`. You need to write your own `Close()` function so you `Flush()` your buffer at the end and not lose data. Second was marker interfaces. Pretty cool for asserting that only a certain type is allowed. There was also Go error etiquette since apparently you want lowercase and no subject with `%w` for error wrapping.

Last thing I want to add was something sneaky when testing race conditions. What was caught was me forgetting to add the mutex lock to `Close()` which was needed since it used the same `closed` and `buf` fields that `Write()` guards... very sneaky.
