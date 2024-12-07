variable "resource_group_location" {
  type        = string
  description = "Location for all resources."
  default     = "westeurope"
}

variable "resource_group_name_prefix" {
  type        = string
  description = "Prefix of the resource group name that's combined with a random ID so name is unique in your Azure subscription."
  default     = "rg"
}

variable "virtual_machines" {
  type = number
  description = "Number of virtual machines to be created"
  default = 2
}

variable "use_btc" {
  type = bool
  description = "Whether to use BTC or BSV blockchain - true: BTC | false: BSV"
  default = true
}

variable "vm_size" {
  type = string
  description = "Azure VM size"
  default = "Standard_A8_v2" # other options: https://learn.microsoft.com/en-us/azure/virtual-machines/sizes/general-purpose/av2-series?tabs=sizebasic
}

variable "broadcaster_version" {
  type = string
  description = "Version of broadcaster which to run"
  default = "v0.1.1"
}

variable "start_time" {
  type = string
  description = "Time at which to start the broadcaster"
  default = "2024-12-07T17:50:00+01:00"
}
