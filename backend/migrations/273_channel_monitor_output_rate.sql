-- 输出速度只记录完整响应的实测值，旧首字即止探测没有可靠用量，不回填。
ALTER TABLE channel_monitor_histories
    ADD COLUMN IF NOT EXISTS output_tokens_per_second DOUBLE PRECISION;
COMMENT ON COLUMN channel_monitor_histories.output_tokens_per_second IS
    '生成阶段输出Token每秒；上游output usage / (完整耗时-首字耗时)，缺样本或失败为NULL';
