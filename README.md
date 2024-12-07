# What is node analysis?

Node anlysis allows to compare BSV & BTC nodes with respect to their performance.

## How to create the infrastructure on Microsoft Azure

Prepare terraform and Azure subscription

1. Install [Terraform](https://developer.hashicorp.com/terraform/install)
2. Create an account & subscription on [Azure](https://azure.microsoft.com/en-us/)

Create the infrastructure

1. Initiate terraform `terraform -chdir=infra init`
2. Create infrastructure
    - for BSV: `terraform -chdir=infra apply -var use_btc=false`
    - for BTC: `terraform -chdir=infra apply -var use_btc=true` (use_btc=true is default)
    - By default a network with 2 VMs is created. In order to have a different number of VMs for example 3, create the infrastructure with an additional variable
        - `terraform -chdir=infra apply -var virtual_machines=3`
    - With the given infrastructure code, the maximum number of VMs is 5

Possibly the quota for `Standard Av2 Family vCPUs` and `Total Regional vCPUs` needs to be increased: https://portal.azure.com/#view/Microsoft_Azure_Capacity/QuotaMenuBlade/~/myQuotas

## Build and deploy node analysis application

To make the pem file executable `make executable`

To build the application run `make build`

Change mode of pem file `chmod 400 ./infra/private_keys/cloudtls.pem`

Deploy the application by running `deploy.sh`

## Connect to instances

Show vm resource ids
```bash
terraform -chdir=infra output -json vm_resource_ids | jq '.[].[]'
```

Show the resource group

```bash
terraform -chdir=infra output -json resource_group_name
```

To connect to a particular VM use the following command while replacing `<vm resource id>` and `<resource group name>` with the output from the respective previous commands

```bash
az network bastion ssh --name bastion_host --resource-group <resource group name> --target-resource-id <vm resource id> --auth-type "ssh-key" --username azureuser --ssh-key ./infra/private_keys/cloudtls.pem
```


## Run the node analysis application

### Start the listener and store logs to file

Run the following command to see the meaning of each flag
```
./listener -h
```

For BSV: 
```
./listener -port=28332 > output.txt
```

For BTC: 
```
./listener > output.txt
```

### Start broadcaster

Run the following command to see the meaning of each flag
```
./broadcaster -h
```


For BSV: 
```
./broadcaster -port=18332 -blockchain=bsv -rate=10 -limit=200 -start-at=2024-12-02T21:16:00+01:00 -gen-blocks=5s
```
For BTC: 
```
./broadcaster -port=18443 -blockchain=btc -rate=10 -limit=200 -start-at=2024-12-02T21:16:00+01:00 -gen-blocks=5s
```
