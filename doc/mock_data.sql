-- ==============================================================================
-- 真实感 Mock 数据填充脚本
-- 使用 PostgreSQL 原生 gen_random_uuid() 和 CTE (Common Table Expressions) 
-- 确保主外键关系正确，同时内容更贴近真实用户场景。
-- 注意：执行此脚本会清空现有表数据！
-- ==============================================================================

-- 1. 清空现有数据
TRUNCATE TABLE project_platform_publications, projects, users CASCADE;

-- 2. 插入用户并将其 UUID 捕获到 CTE 中
WITH inserted_users AS (
    INSERT INTO users (id, username, created_at, updated_at) 
    VALUES 
        (gen_random_uuid(), 'kuroda_kayn', now() - interval '30 days', now()),
        (gen_random_uuid(), 'tech_weekly', now() - interval '15 days', now())
    RETURNING id, username
),

-- 3. 插入项目并捕获其 UUID
inserted_projects AS (
    INSERT INTO projects (id, user_id, title, source_content, status, created_at, updated_at)
    VALUES 
        -- 项目 1: 一篇关于 AI 的深度长文 (已发布)
        (gen_random_uuid(), (SELECT id FROM inserted_users WHERE username = 'kuroda_kayn'), 
         '2026年AI行业大模型发展趋势深度解析', 
         '在过去的几年里，大语言模型（LLM）经历了爆炸式的增长。本文将从多模态、边缘计算以及Agent生态三个维度，探讨2026年及未来的AI发展趋势。首先，多模态模型的融合已经成为行业共识...', 
         'published', now() - interval '3 days', now() - interval '3 days'),
        
        -- 项目 2: 一篇日常科技硬件开箱 (发布中)
        (gen_random_uuid(), (SELECT id FROM inserted_users WHERE username = 'tech_weekly'), 
         '首发评测：苹果Vision Pro二代到底值不值得买？', 
         '终于拿到了期待已久的Vision Pro第二代！这次据说在重量上减轻了30%，而且续航有明显提升。这篇评测我们就来详细聊聊它的佩戴体验以及最新的空间计算应用生态...', 
         'publishing', now() - interval '5 hours', now() - interval '1 hours'),
        
        -- 项目 3: 一篇简短的行业快讯 (发布失败)
        (gen_random_uuid(), (SELECT id FROM inserted_users WHERE username = 'kuroda_kayn'), 
         '突发：OpenAI宣布开源最新一代小型推理模型', 
         '今晨，OpenAI在其官方博客意外宣布，将完全开源其最新研发的7B参数小型推理模型。这一举动预计将对整个开源开源社区产生巨大震动。', 
         'failed', now() - interval '1 days', now() - interval '20 hours'),

        -- 项目 4: 正在草稿箱里的笔记 (草稿)
        (gen_random_uuid(), (SELECT id FROM inserted_users WHERE username = 'tech_weekly'), 
         '下周 WWDC 前瞻预测清单', 
         '整理一下下周苹果WWDC可能发布的内容：1. iOS 20 的 AI 彻底重构... (待补充完善)', 
         'draft', now() - interval '1 hours', now() - interval '10 minutes')
    RETURNING id, title
)

-- 4. 插入平台发布记录，利用刚生成的项目 ID
INSERT INTO project_platform_publications (id, project_id, platform, enabled, status, config, adapted_content, remote_id, publish_url, created_at, updated_at)
VALUES
    -- 项目 1 (AI趋势) 分发到 微信公众号 和 知乎
    (gen_random_uuid(), (SELECT id FROM inserted_projects WHERE title LIKE '%AI行业大模型%'), 'wechat', true, 'published',
     '{"title": "2026年AI行业大模型发展趋势", "tags": ["人工智能", "大模型", "前沿科技"], "original_declaration": true, "author_token": "wechat_auth_112233_SHOULD_BE_HIDDEN"}',
     '{"summary": "万字长文解析2026年AI大模型发展趋势，带你读懂多模态与Agent生态的未来。点击阅读原文获取完整报告。", "format": "html", "full_text": "<p>极长的HTML排版代码...</p>"}',
     'wx_article_998877', 'https://mp.weixin.qq.com/s/abcdefg123456', now() - interval '3 days', now() - interval '3 days'),
     
    (gen_random_uuid(), (SELECT id FROM inserted_projects WHERE title LIKE '%AI行业大模型%'), 'zhihu', true, 'published',
     '{"title": "如何看待2026年AI行业大模型的发展趋势？", "tags": ["人工智能", "深度学习"], "category": "tech", "session_cookie": "zhihu_cookie_xyz_SHOULD_BE_HIDDEN"}',
     '{"summary": "谢邀。关于2026年大模型的发展，我认为核心在于边缘端侧的落地。本文将深度解析...", "format": "markdown", "full_text": "# 2026 AI 趋势\n\n正文内容..."}',
     'zh_answer_554433', 'https://www.zhihu.com/question/12345/answer/67890', now() - interval '3 days', now() - interval '3 days'),

    -- 项目 2 (Vision Pro) 正在分发到 B站 和 小红书
    (gen_random_uuid(), (SELECT id FROM inserted_projects WHERE title LIKE '%Vision Pro%'), 'bilibili', true, 'publishing',
     '{"title": "【首发】苹果Vision Pro 2代全网最详细开箱体验！", "tags": ["数码", "苹果", "VR"], "cover_image": "https://img.example.com/cover/vp2.jpg"}',
     '{"summary": "第二代Vision Pro终于来了！重量减轻、续航增加，它能成为下一代计算平台吗？三连关注，视频即将上线！", "format": "text"}',
     NULL, NULL, now() - interval '1 hours', now()),
     
    (gen_random_uuid(), (SELECT id FROM inserted_projects WHERE title LIKE '%Vision Pro%'), 'xiaohongshu', true, 'pending',
     '{"title": "沉浸式体验🍎Vision Pro 2代，这也太酷了吧！", "tags": ["好物分享", "数码科技", "苹果女孩"]}',
     '{"summary": "终于拿到新玩具啦！佩戴比一代舒服太多了😭... #苹果 #VisionPro", "format": "text"}',
     NULL, NULL, now() - interval '1 hours', now()),

    -- 项目 3 (OpenAI开源) 分发到 微信公众号 失败
    (gen_random_uuid(), (SELECT id FROM inserted_projects WHERE title LIKE '%OpenAI%'), 'wechat', true, 'failed',
     '{"title": "OpenAI突然开源7B模型！", "tags": ["突发", "AI"]}',
     '{"summary": "今晨突发！OpenAI彻底开源小型推理模型...", "format": "html"}',
     NULL, NULL, now() - interval '20 hours', now() - interval '20 hours');
     
-- 为了能让你测试分页，再插入 15 条模拟的历史项目（只插入项目表即可）
DO $$
DECLARE
    user_id_kayn uuid;
BEGIN
    SELECT id INTO user_id_kayn FROM users WHERE username = 'kuroda_kayn' LIMIT 1;
    
    FOR i IN 1..15 LOOP
        INSERT INTO projects (id, user_id, title, source_content, status, created_at, updated_at)
        VALUES (
            gen_random_uuid(), 
            user_id_kayn, 
            '往期文章归档：开发日志 ' || i || ' 期', 
            '这是系统早期测试时沉淀的内容归档...', 
            'published', 
            now() - (i + 10 || ' days')::interval, 
            now() - (i + 10 || ' days')::interval
        );
    END LOOP;
END $$;
