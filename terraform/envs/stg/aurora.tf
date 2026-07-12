# Aurora MySQL 8.0（ソース・ターゲットの2クラスタ）
# DMSのCDCにはbinlog(ROW形式)が必要なため、クラスタパラメータグループで有効化する

resource "aws_db_subnet_group" "main" {
  name       = "${var.name_prefix}-db-subnet"
  subnet_ids = [aws_subnet.private_a.id, aws_subnet.private_c.id]
}

resource "aws_rds_cluster_parameter_group" "source" {
  name   = "${var.name_prefix}-source-params"
  family = "aurora-mysql8.0"

  parameter {
    name         = "binlog_format"
    value        = "ROW"
    apply_method = "pending-reboot"
  }

  parameter {
    name         = "binlog_row_image"
    value        = "full"
    apply_method = "pending-reboot"
  }
}

resource "aws_rds_cluster" "source" {
  cluster_identifier              = "${var.name_prefix}-source"
  engine                          = "aurora-mysql"
  engine_version                  = "8.0.mysql_aurora.3.10.0"
  database_name                   = "source_orders"
  master_username                 = var.db_master_username
  master_password                 = var.db_master_password
  db_subnet_group_name            = aws_db_subnet_group.main.name
  vpc_security_group_ids          = [aws_security_group.aurora.id]
  db_cluster_parameter_group_name = aws_rds_cluster_parameter_group.source.name
  skip_final_snapshot             = true
  backup_retention_period         = 1

  serverlessv2_scaling_configuration {
    min_capacity = 0.5
    max_capacity = 1.0
  }
}

resource "aws_rds_cluster_instance" "source" {
  identifier          = "${var.name_prefix}-source-1"
  cluster_identifier  = aws_rds_cluster.source.id
  engine              = aws_rds_cluster.source.engine
  engine_version      = aws_rds_cluster.source.engine_version
  instance_class      = "db.serverless"
  publicly_accessible = true # 検証用。スキーマ適用とテストデータ投入を作業端末から行う
}

resource "aws_rds_cluster" "target" {
  cluster_identifier      = "${var.name_prefix}-target"
  engine                  = "aurora-mysql"
  engine_version          = "8.0.mysql_aurora.3.10.0"
  database_name           = "target_orders"
  master_username         = var.db_master_username
  master_password         = var.db_master_password
  db_subnet_group_name    = aws_db_subnet_group.main.name
  vpc_security_group_ids  = [aws_security_group.aurora.id]
  skip_final_snapshot     = true
  backup_retention_period = 1

  serverlessv2_scaling_configuration {
    min_capacity = 0.5
    max_capacity = 1.0
  }
}

resource "aws_rds_cluster_instance" "target" {
  identifier          = "${var.name_prefix}-target-1"
  cluster_identifier  = aws_rds_cluster.target.id
  engine              = aws_rds_cluster.target.engine
  engine_version      = aws_rds_cluster.target.engine_version
  instance_class      = "db.serverless"
  publicly_accessible = true # 検証用
}
