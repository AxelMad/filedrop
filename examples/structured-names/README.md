# Example: panels + laptops + all-in-one PCs with structured names

`agent.conf` here maps a structured hostname convention
(`<p|n|m><number>-<site>-<room>-<n>`) onto filedrop's configurable naming
scheme and uses the directory service with a server that has several
addresses. Adapt `HOSTNAME_PATTERN` to your own names; run
`filedrop-agent check` on a machine to see how it is interpreted.
