# terraform {
#   required_version = ">=1.0"
#   required_providers {
#     azurerm = {
#       source  = "hashicorp/azurerm"
#       version = "~>3.0"
#     }
#   }
# }
# provider "azurerm" {
#   features {}
# }


terraform {
  required_providers {
    azapi = {
      source  = "azure/azapi"
      version = "~>1.5"
    }
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.75.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~>3.0"
    }
  }

  required_version = ">= 1.5.7"
}

provider "azurerm" {
  skip_provider_registration = "true" # This is only required when the User, Service Principal, or Identity running Terraform lacks the permissions to register Azure Resource Providers.
  features {}
}
# # Create a resource group
# resource "azurerm_resource_group" "rg-wtf" {
#   name     = "rg-with-terraform"
#   location = "West Europe"
# }