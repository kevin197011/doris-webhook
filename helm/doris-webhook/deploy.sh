#!/bin/bash
# Doris Webhook Helm 部署脚本

helm upgrade --install doris-webhook . -n devops -f values.yaml
