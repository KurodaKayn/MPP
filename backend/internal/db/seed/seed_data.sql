-- Dev-only seed data. This file is executed after GORM AutoMigrate.
-- Keep inserts idempotent so restarting the dev backend does not duplicate rows.

WITH seed_users(id, username, password_hash, created_at, updated_at) AS (
    VALUES
        ('00000000-0000-4000-8000-000000000001'::uuid, 'test_user', '$2a$10$JuGX0AMl3DS3eGm/yRvY2OZLm4QuTuoIgRT4ucmVs/BCwoPYARN4C', now() - interval '1 day', now()),
        ('00000000-0000-4000-8000-000000000002'::uuid, 'kuroda_kayn', '$2a$10$JuGX0AMl3DS3eGm/yRvY2OZLm4QuTuoIgRT4ucmVs/BCwoPYARN4C', now() - interval '30 days', now()),
        ('00000000-0000-4000-8000-000000000003'::uuid, 'tech_weekly', '$2a$10$JuGX0AMl3DS3eGm/yRvY2OZLm4QuTuoIgRT4ucmVs/BCwoPYARN4C', now() - interval '15 days', now())
),
inserted_users AS (
    INSERT INTO users (id, username, password_hash, created_at, updated_at)
    SELECT su.id, su.username, su.password_hash, su.created_at, su.updated_at
    FROM seed_users su
    WHERE NOT EXISTS (
        SELECT 1
        FROM users u
        WHERE u.username = su.username
    )
    RETURNING id, username
),
available_users AS (
    SELECT DISTINCT ON (username) id, username
    FROM (
        SELECT id, username, 0 AS priority
        FROM inserted_users
        UNION ALL
        SELECT u.id, u.username, 1 AS priority
        FROM users u
        JOIN seed_users su ON su.username = u.username
    ) candidates
    ORDER BY username, priority, id
),
seed_projects(id, username, title, source_content, status, created_at, updated_at) AS (
    VALUES
        (
            '00000000-0000-4000-8000-000000000101'::uuid,
            'test_user',
            '测试用户的第一篇草稿',
            '这是用于本地开发登录后的基础测试内容。',
            'draft',
            now() - interval '2 hours',
            now() - interval '2 hours'
        ),
        (
            '00000000-0000-4000-8000-000000000102'::uuid,
            'kuroda_kayn',
            '2026年AI行业大模型发展趋势深度解析',
            '在过去的几年里，大语言模型经历了爆炸式增长。本文从多模态、边缘计算以及 Agent 生态三个维度，探讨未来 AI 发展趋势。',
            'published',
            now() - interval '3 days',
            now() - interval '3 days'
        ),
        (
            '00000000-0000-4000-8000-000000000103'::uuid,
            'tech_weekly',
            '首发评测：苹果 Vision Pro 二代到底值不值得买？',
            '终于拿到了期待已久的 Vision Pro 第二代。这篇评测会聊聊佩戴体验、续航变化以及空间计算应用生态。',
            'publishing',
            now() - interval '5 hours',
            now() - interval '1 hour'
        ),
        (
            '00000000-0000-4000-8000-000000000104'::uuid,
            'kuroda_kayn',
            '突发：OpenAI宣布开源最新一代小型推理模型',
            '今晨，OpenAI 在官方博客宣布开源最新研发的小型推理模型。',
            'failed',
            now() - interval '1 day',
            now() - interval '20 hours'
        )
),
inserted_projects AS (
    INSERT INTO projects (id, user_id, title, source_content, status, created_at, updated_at)
    SELECT sp.id, au.id, sp.title, sp.source_content, sp.status, sp.created_at, sp.updated_at
    FROM seed_projects sp
    JOIN available_users au ON au.username = sp.username
    WHERE NOT EXISTS (
        SELECT 1
        FROM projects p
        WHERE p.id = sp.id
    )
    RETURNING id, title
),
available_projects AS (
    SELECT id, title FROM inserted_projects
    UNION ALL
    SELECT p.id, p.title
    FROM projects p
    JOIN seed_projects sp ON sp.id = p.id
),
seed_publications(
    id,
    project_title,
    platform,
    enabled,
    status,
    config,
    adapted_content,
    remote_id,
    publish_url,
    error_message,
    retry_count,
    created_at,
    updated_at
) AS (
    VALUES
        (
            '00000000-0000-4000-8000-000000000201'::uuid,
            '2026年AI行业大模型发展趋势深度解析',
            'wechat',
            true,
            'published',
            '{"title":"2026年AI行业大模型发展趋势","tags":["人工智能","大模型","前沿科技"],"original_declaration":true}'::jsonb,
            '{"summary":"解析 2026 年 AI 大模型发展趋势，读懂多模态与 Agent 生态的未来。","format":"html"}'::jsonb,
            'wx_article_998877',
            'https://mp.weixin.qq.com/s/abcdefg123456',
            NULL,
            0,
            now() - interval '3 days',
            now() - interval '3 days'
        ),
        (
            '00000000-0000-4000-8000-000000000202'::uuid,
            '2026年AI行业大模型发展趋势深度解析',
            'zhihu',
            true,
            'published',
            '{"title":"如何看待2026年AI行业大模型的发展趋势？","tags":["人工智能","深度学习"]}'::jsonb,
            '{"summary":"核心在于边缘端侧落地和 Agent 生态成熟。","format":"markdown"}'::jsonb,
            'zh_answer_554433',
            'https://www.zhihu.com/question/12345/answer/67890',
            NULL,
            0,
            now() - interval '3 days',
            now() - interval '3 days'
        ),
        (
            '00000000-0000-4000-8000-000000000203'::uuid,
            '首发评测：苹果 Vision Pro 二代到底值不值得买？',
            'douyin',
            true,
            'publishing',
            '{"title":"苹果 Vision Pro 2 代开箱体验","tags":["数码","苹果","VR"]}'::jsonb,
            '{"summary":"重量减轻、续航增加，它能成为下一代计算平台吗？","format":"text"}'::jsonb,
            NULL,
            NULL,
            NULL,
            0,
            now() - interval '1 hour',
            now()
        ),
        (
            '00000000-0000-4000-8000-000000000205'::uuid,
            '突发：OpenAI宣布开源最新一代小型推理模型',
            'wechat',
            true,
            'failed',
            '{"title":"OpenAI突然开源小型推理模型","tags":["突发","AI"]}'::jsonb,
            '{"summary":"OpenAI 开源小型推理模型，社区反响迅速升温。","format":"html"}'::jsonb,
            NULL,
            NULL,
            '模拟发布失败：本地开发环境未配置平台凭证',
            1,
            now() - interval '20 hours',
            now() - interval '20 hours'
        )
)
INSERT INTO project_platform_publications (
    id,
    project_id,
    platform,
    enabled,
    status,
    config,
    adapted_content,
    remote_id,
    publish_url,
    error_message,
    retry_count,
    created_at,
    updated_at
)
SELECT
    sp.id,
    ap.id,
    sp.platform,
    sp.enabled,
    sp.status,
    sp.config,
    sp.adapted_content,
    sp.remote_id,
    sp.publish_url,
    sp.error_message,
    sp.retry_count,
    sp.created_at,
    sp.updated_at
FROM seed_publications sp
JOIN available_projects ap ON ap.title = sp.project_title
ON CONFLICT DO NOTHING;
