# Submission #2 PoC


### Mission

Implement a small PoC project in Go.


Aim to implement a Load Balancer for multiple nodes with the following rate limits:

- Each node can have a different rate limit.
-  Rate limits are measured in two ways: BPM (http body Bytes Per Minute), RPM
(Requests Per Minute)
-  Rate limits can be hit across any of the options depending on what occurs first