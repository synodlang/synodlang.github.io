# Tribute

Synod is named after a protocol, and the protocol has an author. This page says
who he is, what he wrote, and why we chose the name.

## The paper

In 1990 Leslie Lamport submitted a paper called "The Part-Time Parliament". It
appeared in print in 1998, in *ACM Transactions on Computer Systems* (volume 16,
issue 2, pages 133 to 169). It does not read like a usual systems paper. It tells
the story of the parliament of Paxos, an invented island of ancient Greece, whose
legislators wander in and out of the chamber, yet must still agree on one decree
at a time and never contradict an earlier one.

Hidden in the story is a precise answer to a hard question: how can a group of
machines that may crash, restart and lose messages still agree on a value? The
paper calls its core algorithm the Synod protocol. Today most people call the
whole family of ideas Paxos, and a great deal of the software that keeps data safe
across many machines descends from it.

## Why the name

A language for watching distributed systems think needed a name that stands for
agreement under difficulty. The word *synod* means a council that meets to decide
together, and it is the name Lamport gave to the heart of his algorithm. We took
it as a thank-you and as a reminder of what the language is for: making that kind
of reasoning something you can see.

The `paxos` example in the [examples](docs:examples/paxos/) is the namesake
algorithm written in Synod, so you can watch two proposers race and see why the
chosen value never changes.

## More from the same author

- "Paxos Made Simple" (*ACM SIGACT News*, 2001) explains the algorithm in plain
  prose, for readers who found the parliament story hard to follow.
- "Time, Clocks, and the Ordering of Events in a Distributed System" (*Communications
  of the ACM*, 1978) gave us the vocabulary of events, causality and logical time.
- TLA+ is his specification language for describing and checking systems before
  they are built.
- In 2013 he received the ACM A.M. Turing Award for fundamental contributions to
  the theory and practice of distributed and concurrent systems.

We are grateful for this work. If Synod makes one of its ideas easier to watch and
understand, it has done its job.

## References

1. Leslie Lamport. "The Part-Time Parliament". *ACM Transactions on Computer
   Systems* 16(2), May 1998, 133-169. [ACM Digital Library](https://dl.acm.org/doi/10.1145/279227.279229).
2. Leslie Lamport. "Paxos Made Simple". *ACM SIGACT News* 32(4), December 2001, 51-58.
3. Leslie Lamport. "Time, Clocks, and the Ordering of Events in a Distributed
   System". *Communications of the ACM* 21(7), July 1978, 558-565.
4. [Leslie Lamport's own page of his writings](https://lamport.azurewebsites.net/pubs/pubs.html),
   where his papers can be read, with his notes on how each came to be.
5. [ACM A.M. Turing Award 2013](https://amturing.acm.org/award_winners/lamport_1205376.cfm).
