# FT Monitor

## Introduction
FT Monitor is one of the key actors in a Firmware Transparency World. Its main job is to closely observe the updates that happen in a Firmware Transparency Log and flag any inappropriate log behaviour. This role can be played by the Firmware Vendor who may decide to closely track the operations in a FT Log. Alternatively, it can be an independent entity acting as a Verifier for the released firmware submission and verifies the updates to a FT log for each firmware release.

## Monitor Verification

Following are the sequence of actions which an example FT monitor does: 

* Monitor polls the FT log according to configured interval, to fetch latest checkpoint (i. e. most up to date index)  
* It then fetches the consistency proof between its previous stored checkpoint and the new checkpoint (as above) and verifies
  locally that the head of the tree is consistent with previous head. 
* It then iterates through each entry (from previously stored to latest checkpoint index) to fetch the Manifest statement (stored
  in the log) and its inclusion proof by querying the log. 
* Decodes Firmware Hash from the Manifest statement
* Fetches the firmware, using firmware hash as index from a separately stored Firmware Database, known as Content Addressable 
  Store (CAS).
* Hashes the fetched Firmware Image to compare the same with the hash received from Manifest
* Checks the Firmware Image for any Malware keyword mathces. If matches, can mark the annotation into the log indicating that the 
  found firmware is not good.
* Once all the entries are verified as above, remembers the most up to date entry in a store (file as an example).
  This saves monitor from reverifying all the entries in case it needs a restart.


## Example Workflow

Run the FT Monitor as indicated in the [Firmware Transparency](../../../firmware)

```bash
# One can supplu following optional arguments to control the various configuration parameters that are 
# baked into the monitor operation.  
1. Poll interval in seconds i. e. the time window when monitor wakes up to look for new entries in the log, example -- poll_interval = 20
2. Keywords to look for the in the binary, example -- keyword = trojan 
3. Add annotations to the log in addtion to local logging, example -- annotate=true
4. Persist Monitor state by supplying a file argument, example --file_path = /tmp/mon_file.db
```