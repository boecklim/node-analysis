# What is node analysis?

Node analysis is tool which allows to run simulated networks of BSV & BTC nodes with a broadcaster application that ingests transactions at a specified rate while logging the frequency and size of blocks generated by simulated mining.

## How to run

Prepare terraform and Azure subscription

1. Install [Terraform](https://developer.hashicorp.com/terraform/install)
2. Create an account & subscription on [Azure](https://azure.microsoft.com/en-us/)

Both the infrastructure and the deployment of broadcaster can be done in one Go. The flags for the broadcaster application are parameterized and can be given together with the command which creates the infrastructure

For BTC:
```bash
terraform -chdir=infra destroy --auto-approve -var use_btc=true -var virtual_machines=5 -var broadcaster_version=v0.1.12 -var rate=50 -var limit=1h -var gen_block_time=10m -var start_time="2024-12-10T21:58:00+01:00"
```

For BSV:
```bash
terraform -chdir=infra destroy --auto-approve -var use_btc=false -var virtual_machines=5 -var broadcaster_version=v0.1.12 -var rate=50 -var limit=1h -var gen_block_time=10m -var start_time="2024-12-10T21:58:00+01:00"
```

Possibly the quota for `Standard Av2 Family vCPUs` and `Total Regional vCPUs` needs to be increased: https://portal.azure.com/#view/Microsoft_Azure_Capacity/QuotaMenuBlade/~/myQuotas


## Connect to instances

In order to connect to any of the instances run the following script
```bash
./connect.sh <nr of instance starting with 0>
```

There is also a script which connects to 5 nodes (if 5 have been setup) each in a different [tmux](https://github.com/tmux/tmux/wiki) pane:

```bash
./tmux_session.sh
```

## Run the node analysis application

### Start broadcaster

Run the following command to see the meaning of each flag
```
./broadcaster -h
```

For BSV: 
```
./broadcaster -rpc-port=18332 -zmq-port=28332 -blockchain=bsv -gen-blocks=15s -rate=10 -limit=2m -output=./results/bsv/output.log -start-at=2024-12-11T13:30:00+01:00
```

For BTC: 
```
./broadcaster -rpc-port=18443 -zmq-port=29000 -blockchain=btc -gen-blocks=10s -rate=10 -limit=2m -output=./results/btc/output.log -start-at=2024-12-09T17:56:00+01:00

```
