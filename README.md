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
    - for BTC: `terraform -chdir=infra apply -var use_btc=true`

## Build and deploy node analysis application

To build the application run `make build`

Deploy the application by running `deploy.sh`
