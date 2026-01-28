-- 为 video_metrics 表添加时间字段
-- 如果表已经有时间字段，可以跳过此步骤

-- 方案 1: 添加 event_time 字段（推荐）
ALTER TABLE video_metrics ADD COLUMN event_time DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '事件时间';

-- 如果表已有数据，需要为现有数据设置时间（使用当前时间或导入时间）
-- UPDATE video_metrics SET event_time = CURRENT_TIMESTAMP WHERE event_time IS NULL;

-- 方案 2: 如果 Doris 支持，可以使用导入时间作为时间字段
-- 这需要查看 Doris 的导入时间函数，通常可以通过 __DORIS_DELETE_SIGN__ 或其他系统字段获取

-- 验证表结构
DESC video_metrics;
