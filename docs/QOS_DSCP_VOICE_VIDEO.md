# QoS / DSCP enrichment for voice and video

This POC treats QoS as a first-class enrichment dimension, with special emphasis on voice and video.

## What is extracted

NetFlow v5 carries the IPv4 ToS byte. DSCP is derived as:

```text
dscp = tos >> 2
ecn  = tos & 0x03
```

The connector emits these attributes on represented traces:

```text
network.tos
network.dscp
network.dscp.class
network.ecn
qos.class
```

## Voice and video classes

The default mappings are:

```text
DSCP 46 / EF   -> qos.class=voice
DSCP 40 / CS5  -> qos.class=voice_signaling
DSCP 32 / CS4  -> qos.class=video
DSCP 34 / AF41 -> qos.class=video
DSCP 36 / AF42 -> qos.class=video
DSCP 38 / AF43 -> qos.class=video
```

## Metrics added

QoS is intentionally emitted on bounded and useful metric families only:

```text
flow2span.link.qos.bits_per_second
flow2span.link.qos.utilization_percent
flow2span.community_dependency.bits_per_second
flow2span.top_talker.bits_per_second
flow2span.top_talker.bytes
flow2span.top_talker.packets
```

QoS dimensions:

```text
network.dscp
network.dscp.class
qos.class
```

This lets the customer answer:

```text
How much of each WAN link is voice?
How much is video?
Which top talkers are consuming EF or AF41 queues?
Are voice/video markings preserved end-to-end?
Which site/community is driving real-time media utilization?
```

## Cardinality guidance

Do not add DSCP to broad site rollup metrics by default. Keep DSCP on:

```text
link.qos.*
community_dependency.*
top_talker.*
represented traces
```

That gives QoS visibility without exploding metric series across all site/application/link combinations.

## Generator behavior

The bundled NetFlow v5 generator now emits synthetic voice and video flows:

```text
EF / DSCP 46 voice RTP-like UDP
AF41 / DSCP 34 video-like UDP
AF31 / DSCP 26 business-critical sample
CS0 / DSCP 0 best-effort sample
```
