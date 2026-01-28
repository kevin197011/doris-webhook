# Grafana Dashboard 设置指南

## 前置条件

1. **Grafana 已安装并运行**
2. **已配置 MySQL 数据源连接到 Doris**
   - Doris 支持 MySQL 协议，可以直接使用 MySQL 数据源
   - 连接地址：`<doris-fe-host>:9030`（FE 查询端口）
   - 用户名和密码：与 Doris 登录凭证相同

## 步骤 1: 添加时间字段（推荐）

为了支持时间序列图表，建议为表添加时间字段：

```sql
-- 连接到 Doris
mysql -h <doris-fe-host> -P 9030 -u <username> -p

-- 添加时间字段
ALTER TABLE video_metrics ADD COLUMN event_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '事件时间';

-- 为现有数据设置时间（可选）
UPDATE video_metrics SET event_time = CURRENT_TIMESTAMP WHERE event_time IS NULL;
```

**注意**：代码已更新，新写入的数据会自动包含 `event_time` 字段。

## 步骤 2: 配置 Grafana 数据源

1. 登录 Grafana
2. 进入 **Configuration** → **Data Sources**
3. 点击 **Add data source**
4. 选择 **MySQL**
5. 配置连接信息：
   - **Host**: `<doris-fe-host>:9030`
   - **Database**: `video`（或你的数据库名）
   - **User**: Doris 用户名
   - **Password**: Doris 密码
   - **TLS/SSL Mode**: Disable（除非启用了 SSL）
6. 点击 **Save & Test**

## 步骤 3: 导入 Dashboard

### 方案 A: 使用时间序列 Dashboard（推荐，需要时间字段）

1. 进入 **Dashboards** → **Import**
2. 点击 **Upload JSON file**
3. 选择 `grafana-dashboard.json`
4. 选择数据源（刚才配置的 MySQL 数据源）
5. 点击 **Import**

### 方案 B: 使用简化 Dashboard（无需时间字段）

如果表还没有时间字段，可以使用简化版本：

1. 进入 **Dashboards** → **Import**
2. 点击 **Upload JSON file**
3. 选择 `grafana-dashboard-simple.json`
4. 选择数据源
5. 点击 **Import**

## Dashboard 功能说明

### 时间序列版本（grafana-dashboard.json）

包含以下面板：

1. **总事件数 - 时间序列**
   - 显示事件数随时间的变化趋势
   - 支持时间范围选择

2. **按项目分组 - 事件数**
   - 不同项目的事件数趋势对比
   - 堆叠面积图

3. **按事件类型分组 - 事件数**
   - 不同事件类型的事件数趋势
   - 堆叠面积图

4. **项目分布**（饼图）
   - 各项目的事件占比

5. **事件类型分布**（饼图）
   - 各事件类型的占比

6. **总事件数**（统计卡片）
   - 当前时间范围内的总事件数

7. **项目事件统计**（表格）
   - 每个项目的详细统计

8. **事件类型统计**（表格）
   - 每个事件类型的详细统计

9. **项目 x 事件类型 - 热力图**
   - 项目与事件类型的组合热力图

### 简化版本（grafana-dashboard-simple.json）

包含以下面板：

1. **项目分布**（饼图）
2. **事件类型分布**（饼图）
3. **总事件数**（统计卡片）
4. **项目数**（统计卡片）
5. **事件类型数**（统计卡片）
6. **唯一用户代理数**（统计卡片）
7. **项目事件统计**（柱状图）
8. **事件类型统计**（柱状图）
9. **项目详细统计**（表格）
10. **事件类型详细统计**（表格）
11. **项目 x 事件类型交叉统计**（表格）

## 变量（Variables）

两个 Dashboard 都包含以下变量，可以在面板顶部进行筛选：

- **project**: 项目筛选（支持多选）
- **event**: 事件类型筛选（支持多选）

## 自定义查询示例

如果需要自定义查询，可以参考以下 SQL 模板：

### 时间序列查询
```sql
SELECT
  DATE_FORMAT(event_time, '%Y-%m-%d %H:%i:%s') as time,
  COUNT(*) as value
FROM video_metrics
WHERE $__timeFilter(event_time)
  AND project IN ($project)
  AND event IN ($event)
GROUP BY time
ORDER BY time
```

### 统计查询
```sql
SELECT
  project,
  event,
  COUNT(*) as count
FROM video_metrics
WHERE $__timeFilter(event_time)
  AND project IN ($project)
  AND event IN ($event)
GROUP BY project, event
ORDER BY count DESC
```

## 常见问题

### 1. 数据源连接失败

**问题**：无法连接到 Doris

**解决方案**：
- 检查 FE 端口是否正确（默认 9030）
- 检查网络连通性
- 确认用户名密码正确
- 检查 Doris 是否允许远程连接

### 2. 时间字段不存在

**问题**：导入时间序列 Dashboard 后报错

**解决方案**：
- 使用 `grafana-dashboard-simple.json`（不需要时间字段）
- 或者执行 `add_timestamp_column.sql` 添加时间字段

### 3. 查询性能慢

**问题**：Dashboard 加载缓慢

**解决方案**：
- 减少时间范围
- 添加索引：`CREATE INDEX idx_event_time ON video_metrics(event_time)`
- 使用物化视图（Doris 支持）

### 4. 时区问题

**问题**：时间显示不正确

**解决方案**：
- 在 Dashboard 设置中调整时区
- 或者在 SQL 中使用 `CONVERT_TZ()` 函数转换时区

## 性能优化建议

1. **添加索引**
   ```sql
   -- 为时间字段添加索引
   ALTER TABLE video_metrics ADD INDEX idx_event_time(event_time);
   
   -- 为项目字段添加索引
   ALTER TABLE video_metrics ADD INDEX idx_project(project);
   
   -- 为事件类型添加索引
   ALTER TABLE video_metrics ADD INDEX idx_event(event);
   ```

2. **使用物化视图**（Doris 特性）
   ```sql
   -- 创建按小时聚合的物化视图
   CREATE MATERIALIZED VIEW video_metrics_hourly AS
   SELECT
     DATE_FORMAT(event_time, '%Y-%m-%d %H:00:00') as hour,
     project,
     event,
     COUNT(*) as count
   FROM video_metrics
   GROUP BY hour, project, event;
   ```

3. **设置数据保留策略**
   - 定期清理旧数据
   - 使用 Doris 的分区表功能

## 更新日志

- **2026-01-28**: 初始版本
  - 添加时间字段支持
  - 创建时间序列 Dashboard
  - 创建简化 Dashboard
