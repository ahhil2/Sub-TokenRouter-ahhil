-- 价格设置统一归属价格配置；旧分组值不复制。升级前停止旧实例。
ALTER TABLE pricing_configs
 ADD COLUMN IF NOT EXISTS peak_rate_enabled boolean DEFAULT false NOT NULL,
 ADD COLUMN IF NOT EXISTS peak_start varchar(5) DEFAULT '' NOT NULL,
 ADD COLUMN IF NOT EXISTS peak_end varchar(5) DEFAULT '' NOT NULL,
 ADD COLUMN IF NOT EXISTS peak_rate_multiplier numeric(10,4) DEFAULT 1 NOT NULL,
 ADD COLUMN IF NOT EXISTS long_context_pricing_enabled boolean DEFAULT true NOT NULL,
 ADD COLUMN IF NOT EXISTS free_openai_fast boolean DEFAULT false NOT NULL,
 ADD COLUMN IF NOT EXISTS batch_image_discount_multiplier numeric(10,4) DEFAULT 0.5 NOT NULL,
 ADD COLUMN IF NOT EXISTS batch_image_hold_multiplier numeric(10,4) DEFAULT 0.6 NOT NULL,
 ADD COLUMN IF NOT EXISTS web_search_price_per_call numeric(20,8),
 ADD COLUMN IF NOT EXISTS search_price_per_1k numeric(20,8),
 ADD COLUMN IF NOT EXISTS audio_realtime_price_per_min numeric(20,8),
 ADD COLUMN IF NOT EXISTS audio_tts_price_per_million_chars numeric(20,8),
 ADD COLUMN IF NOT EXISTS audio_stt_price_per_hour numeric(20,8);

ALTER TABLE groups
 DROP COLUMN IF EXISTS peak_rate_enabled,
 DROP COLUMN IF EXISTS peak_start,
 DROP COLUMN IF EXISTS peak_end,
 DROP COLUMN IF EXISTS peak_rate_multiplier,
 DROP COLUMN IF EXISTS long_context_pricing_enabled,
 DROP COLUMN IF EXISTS free_openai_fast,
 DROP COLUMN IF EXISTS batch_image_discount_multiplier,
 DROP COLUMN IF EXISTS batch_image_hold_multiplier,
 DROP COLUMN IF EXISTS web_search_price_per_call,
 DROP COLUMN IF EXISTS search_price_per_1k,
 DROP COLUMN IF EXISTS audio_realtime_price_per_min,
 DROP COLUMN IF EXISTS audio_tts_price_per_million_chars,
 DROP COLUMN IF EXISTS audio_stt_price_per_hour,
 DROP COLUMN IF EXISTS model_pricing;
