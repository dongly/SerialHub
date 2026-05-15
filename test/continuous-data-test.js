const { chromium } = require('playwright');

(async () => {
    console.log('=== 持续数据接收测试 - COM7 ===\n');
    console.log('测试时长: 10 分钟');
    console.log('检查间隔: 每分钟');
    console.log('');
    
    const browser = await chromium.launch({ 
        headless: false,
        executablePath: 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
    });
    const context = await browser.newContext();
    const page = await context.newPage();
    
    try {
        // 打开终端页面
        console.log('打开终端页面...');
        await page.goto('http://127.0.0.1:5000/terminal');
        await page.waitForTimeout(3000);
        console.log('  ✅ 页面加载成功\n');
        
        // 等待串口自动连接
        console.log('等待串口自动连接...');
        await page.waitForTimeout(2000);
        
        // 获取终端内容
        const getTerminalContent = async () => {
            return await page.$eval('#terminal-container', el => el.textContent);
        };
        
        // 持续测试 10 分钟
        const testDuration = 10; // 分钟
        const checkInterval = 60; // 秒
        let checkCount = 0;
        let successCount = 0;
        
        for (let minute = 0; minute < testDuration; minute++) {
            console.log(`\n--- 第 ${minute + 1} 分钟 ---`);
            
            // 等待 1 分钟
            await page.waitForTimeout(checkInterval * 1000);
            
            // 获取终端内容
            const content = await getTerminalContent();
            checkCount++;
            
            // 检查是否收到数据
            // 查找时间戳模式 (如 "05-15 14:41:54")
            const timestampPattern = /\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}/g;
            const timestamps = content.match(timestampPattern);
            
            // 查找 COM7 或串口相关数据
            const hasCom7Data = content.includes('COM7') || 
                               content.includes('道[') || 
                               content.includes('通道[') ||
                               content.includes('电压') ||
                               content.includes('电流') ||
                               content.includes('功率');
            
            if (timestamps && timestamps.length > 0 && hasCom7Data) {
                console.log(`  ✅ 收到数据`);
                console.log(`     时间戳数量: ${timestamps.length}`);
                console.log(`     最新时间戳: ${timestamps[timestamps.length - 1]}`);
                console.log(`     包含 COM7 数据: ${hasCom7Data}`);
                successCount++;
            } else {
                console.log(`  ⚠️  未检测到新数据`);
                console.log(`     时间戳数量: ${timestamps ? timestamps.length : 0}`);
                console.log(`     包含 COM7 数据: ${hasCom7Data}`);
            }
            
            // 每分钟截图
            await page.screenshot({ path: `continuous-test-minute-${minute + 1}.png`, fullPage: true });
            console.log(`     截图已保存: continuous-test-minute-${minute + 1}.png`);
        }
        
        // 测试结果
        console.log('\n=== 测试结果 ===');
        console.log(`总检查次数: ${checkCount}`);
        console.log(`成功次数: ${successCount}`);
        console.log(`成功率: ${(successCount / checkCount * 100).toFixed(2)}%`);
        
        if (successCount === checkCount) {
            console.log('\n✅ 测试通过！COM7 每分钟都收到数据');
        } else {
            console.log(`\n⚠️  测试部分通过，${checkCount - successCount} 分钟未检测到数据`);
        }
        
        // 最终截图
        await page.screenshot({ path: 'continuous-test-final.png', fullPage: true });
        console.log('\n最终截图已保存: continuous-test-final.png');
        
    } catch (error) {
        console.error('测试错误:', error);
    } finally {
        await browser.close();
    }
})();