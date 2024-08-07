terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.16"
    }
  }

  required_version = ">= 1.2.0"
}

provider "aws" {
  region  = "eu-west-3"
}

resource "aws_vpc" "skhoury_vpc" {
  tags = {
    Name = "skhoury_vpc"

  }
  cidr_block = "10.0.0.0/16"
}

resource "aws_subnet" "skhoury_subnet" {
  vpc_id            = aws_vpc.skhoury_vpc.id
  cidr_block        = "10.0.1.0/24" # Specify the CIDR block for the subnet
  availability_zone = "eu-west-3a" # Adjust to your desired availability zone
  map_public_ip_on_launch = true

  tags = {
    Name = "skhoury_subnet"
  }
}

resource "aws_internet_gateway" "skhoury_igw" {
  vpc_id = aws_vpc.skhoury_vpc.id

  tags = {
    Name = "skhoury_igw"
  }
}

resource "aws_route" "skhoury_route_igw" {
  count = "1"
  route_table_id         = "${aws_vpc.skhoury_vpc.default_route_table_id}"
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = "${aws_internet_gateway.skhoury_igw.id}"

  timeouts {
    create = "5m"
  }
}

resource "aws_security_group" "skhoury_sg" {
  name        = "allow_ssh_access"
  description = "Allow SSH inbound traffic"
  vpc_id = aws_vpc.skhoury_vpc.id

  # Allow SSH ingress from a specific IP
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["88.169.180.212/32"]
  }

  # Allow all egress traffic
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "AllowSSH"
  }
}
resource "aws_instance" "skhoury_ec2" {
  ami           = "ami-0574a94188d1b84a1"#ami-01e466af77a95b93d" # Replace with your specific AMI ID
  instance_type = "t2.large" # Choose the instance type that fits your needs
  subnet_id = aws_subnet.skhoury_subnet.id
  vpc_security_group_ids = ["${aws_security_group.skhoury_sg.id}"]
  # Optional configurations
  key_name               = "cid" # Replace with your key pair name
  associate_public_ip_address = true # Assign a public IP address
  root_block_device {
    volume_size = 750 
    volume_type = "gp2" # Optional: Specify the volume type, e.g., gp2 for General Purpose SSD
  }
  user_data = "${file("user_data.sh")}"

  tags = {
    Name = "skhoury_ec2"
  }
}