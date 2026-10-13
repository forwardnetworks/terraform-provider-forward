# Copyright (c) HashiCorp, Inc.

# Register a collector and attach it to a network. The token is what the
# collector runs with (the Docker collector's TOKEN); Forward shows it only
# here, at registration.
resource "forward_collector" "aws" {
  name = "aws-us-east-1"
}

resource "forward_collector_attachment" "prod" {
  network_id = "123456"
  username   = forward_collector.aws.username
}

resource "aws_secretsmanager_secret_version" "collector_token" {
  secret_id     = aws_secretsmanager_secret.collector_token.id
  secret_string = forward_collector.aws.token
}
