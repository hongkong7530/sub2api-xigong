-- 累计充值包含兑换余额及管理员实际正向入账；退款恢复不是新充值。
-- 只提高累计字段到有记录的入账下界，不反推余额，不减去已计入的其他来源。
-- GREATEST 使重复执行幂等，并保留并发新入账已经增加的累计值。
WITH recorded AS (
    SELECT used_by AS user_id, SUM(value) AS total
    FROM redeem_codes
    WHERE status = 'used' AND used_by IS NOT NULL AND value > 0
      AND type IN ('balance', 'admin_balance')
    GROUP BY used_by
)
UPDATE users AS u
SET total_recharged = GREATEST(u.total_recharged, recorded.total),
    updated_at = NOW()
FROM recorded
WHERE u.id = recorded.user_id AND u.total_recharged < recorded.total;
